// This file adds an editing layer on top of JSONC parsing: a Document that
// remembers where every value came from, so a read-modify-write can change
// only what fleet owns and re-render everything else byte for byte —
// comments, unknown keys, whitespace, and key order included.
//
// The model: containers (objects and arrays) hold entries that each own a
// raw source slice. Untouched entries re-render from their original slice;
// edits replace or remove whole slices and never reformat the survivors.
// New values are inserted as compact generated JSON.

package jsonc

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Document is a parsed JSONC (or plain JSON) document that can be edited
// and re-rendered.
type Document struct {
	root  value
	trail string // whitespace after the root value (e.g. the final newline)
}

// Parse parses JSONC text into an editable Document. Comments and trailing
// commas are allowed and preserved on render.
func Parse(src string) (*Document, error) {
	p := &parser{src: src}
	v, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	trail := p.src[p.pos:]
	if rest := strings.TrimSpace(trail); rest != "" {
		return nil, fmt.Errorf("unexpected trailing content %q", truncateForError(rest))
	}
	return &Document{root: v, trail: trail}, nil
}

// ParseStrict is Parse for files whose harness requires strict JSON: it
// rejects comments (and, since the input must already be valid JSON,
// trailing commas) so a strict writer never emits them.
func ParseStrict(src string) (*Document, error) {
	p := &parser{src: src, strict: true}
	v, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	trail := p.src[p.pos:]
	if rest := strings.TrimSpace(trail); rest != "" {
		return nil, fmt.Errorf("unexpected trailing content %q", truncateForError(rest))
	}
	if v.hasComments() {
		return nil, fmt.Errorf("strict JSON required: comments are not allowed here")
	}
	if v.hasTrailingComma() {
		return nil, fmt.Errorf("strict JSON required: trailing commas are not allowed here")
	}
	return &Document{root: v, trail: trail}, nil
}

// RootObject returns the root Object, or nil when the document's root is
// not an Object.
func (d *Document) RootObject() *Object {
	o, _ := d.root.(*Object)
	return o
}

// Render returns the document's current text, including any trailing
// whitespace the source had after the root value.
func (d *Document) Render() string {
	var b strings.Builder
	d.root.render(&b)
	b.WriteString(d.trail)
	return b.String()
}

func truncateForError(s string) string {
	if len(s) > 20 {
		return s[:20] + "..."
	}
	return s
}

// value is one JSON value in the document tree.
type value interface {
	// render appends the value's current text to b.
	render(b *strings.Builder)
	// raw returns the value's current text.
	raw() string
	// hasComments reports whether any comment survives inside the value.
	hasComments() bool
	// hasTrailingComma reports whether the value ends an Array or Object
	// with a comma before the closing bracket.
	hasTrailingComma() bool
}

// literal is a scalar or generated value held as raw text.
type literal struct {
	// text is the source span (or generated JSON) rendered verbatim.
	text string
	// generated marks values fleet created; they carry no source span and
	// can never contain comments.
	generated bool
}

func (l *literal) render(b *strings.Builder) { b.WriteString(l.text) }
func (l *literal) raw() string               { return l.text }
func (l *literal) hasComments() bool         { return !l.generated && hasComment(l.text) }
func (l *literal) hasTrailingComma() bool    { return false }

// entry is one key/value pair of an Object or one item of an Array.
type entry struct {
	// lead is the raw source text between the previous item (or the opening
	// brace) and this key or value: whitespace and comments, verbatim.
	lead string
	// key is the decoded Object key; keyRaw its quoted source form. Empty
	// for Array items.
	key, keyRaw string
	// colon is the raw text between key and value (usually ": ").
	colon string
	// val is the entry's value. For generated entries it is compact JSON.
	val value
	// commaSep is the raw text from the value's end through a following
	// comma ("" when no comma).
	commaSep string
}

func (e *entry) render(b *strings.Builder) {
	b.WriteString(e.lead)
	if e.keyRaw != "" {
		b.WriteString(e.keyRaw)
		b.WriteString(e.colon)
	}
	e.val.render(b)
	b.WriteString(e.commaSep)
}

// Object is a JSON Object that remembers entry order and raw slices.
type Object struct {
	entries   []entry
	closeLead string // raw text between the last item and '}'
	// strict marks objects parsed by ParseStrict: edits keep the result
	// valid strict JSON (no leftover trailing commas).
	strict bool
}

func (o *Object) render(b *strings.Builder) {
	b.WriteString("{")
	for i := range o.entries {
		o.entries[i].render(b)
	}
	b.WriteString(o.closeLead)
	b.WriteString("}")
}

func (o *Object) raw() string {
	var b strings.Builder
	o.render(&b)
	return b.String()
}

func (o *Object) hasComments() bool {
	if hasComment(o.closeLead) {
		return true
	}
	for i := range o.entries {
		e := &o.entries[i]
		if hasComment(e.lead) || hasComment(e.colon) || hasComment(e.commaSep) {
			return true
		}
		if e.val.hasComments() {
			return true
		}
	}
	return false
}

// hasComment reports whether s contains a JSONC comment outside string
// literals.
func hasComment(s string) bool {
	inString, escaped := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '/':
			if i+1 < len(s) && (s[i+1] == '/' || s[i+1] == '*') {
				return true
			}
		}
	}
	return false
}

// get returns the entry with the given key, or nil.
func (o *Object) get(key string) *entry {
	for i := range o.entries {
		if o.entries[i].key == key {
			return &o.entries[i]
		}
	}
	return nil
}

// Has reports whether the key is present.
func (o *Object) Has(key string) bool { return o.get(key) != nil }

// Keys returns the Object's keys in source order.
func (o *Object) Keys() []string {
	keys := make([]string, len(o.entries))
	for i := range o.entries {
		keys[i] = o.entries[i].key
	}
	return keys
}

// KV is one Object entry as seen by callers: the decoded key and the
// value's raw JSON text.
type KV struct {
	Key      string
	RawValue string
}

// KVs returns every entry as a KV, in source order.
func (o *Object) KVs() []KV {
	kvs := make([]KV, len(o.entries))
	for i := range o.entries {
		kvs[i] = KV{Key: o.entries[i].key, RawValue: o.entries[i].val.raw()}
	}
	return kvs
}

// String returns the value stored at key decoded as a JSON string, with
// ok=false when the key is absent or holds a non-string.
func (o *Object) String(key string) (string, bool) {
	e := o.get(key)
	if e == nil {
		return "", false
	}
	var s string
	if err := json.Unmarshal([]byte(e.val.raw()), &s); err != nil {
		return "", false
	}
	return s, true
}

// Obj returns the value stored at key as an Object, or nil when the key is
// absent or holds a non-Object.
func (o *Object) Obj(key string) *Object {
	e := o.get(key)
	if e == nil {
		return nil
	}
	sub, _ := e.val.(*Object)
	return sub
}

// Array returns the value stored at key as an Array, or nil when the key
// is absent or holds a non-Array.
func (o *Object) Array(key string) *Array {
	e := o.get(key)
	if e == nil {
		return nil
	}
	sub, _ := e.val.(*Array)
	return sub
}

func (o *Object) hasTrailingComma() bool {
	if n := len(o.entries); n > 0 && o.entries[n-1].commaSep != "" {
		return true
	}
	for i := range o.entries {
		if o.entries[i].val.hasTrailingComma() {
			return true
		}
	}
	return false
}

// Set assigns key a value given as JSON text (compact). An existing entry
// keeps its lead, key, and colon; only the value is replaced. A new entry
// is appended last with synthesized formatting that mimics the file.
func (o *Object) Set(key, rawValue string) {
	if e := o.get(key); e != nil {
		e.val = &literal{text: rawValue, generated: true}
		return
	}
	o.append(entry{lead: o.newEntryLead(), key: key, keyRaw: quoteJSON(key), colon: ": ", val: &literal{text: rawValue, generated: true}})
}

// Delete removes the entry with the given key. Comments attached before
// the entry go with it. Deleting the last entry strips the previous
// entry's comma when the document is strict.
func (o *Object) Delete(key string) bool {
	for i := range o.entries {
		if o.entries[i].key != key {
			continue
		}
		wasLast := i == len(o.entries)-1
		o.entries = append(o.entries[:i], o.entries[i+1:]...)
		if wasLast && o.strict {
			if n := len(o.entries); n > 0 {
				o.entries[n-1].commaSep = ""
			}
		}
		return true
	}
	return false
}

// append adds an entry at the end, giving the previous last entry its
// comma when it lacked one. When the file's own style ends with a trailing
// comma, the new last entry keeps that style.
func (o *Object) append(e entry) {
	if n := len(o.entries); n > 0 {
		switch {
		case o.entries[n-1].commaSep == "":
			o.entries[n-1].commaSep = ","
		case strings.HasSuffix(o.entries[n-1].commaSep, ","):
			e.commaSep = "," // trailing-comma style
		}
	}
	o.entries = append(o.entries, e)
}

// newEntryLead synthesizes the lead for a new entry from the file's own
// style: the previous entry's indentation when the Object is multi-line, a
// single space for later entries in an inline Object, and nothing for the
// first entry in an inline Object.
func (o *Object) newEntryLead() string {
	if n := len(o.entries); n > 0 {
		prev := o.entries[n-1].lead
		if strings.Contains(prev, "\n") {
			return "\n" + indentOf(prev)
		}
		return " "
	}
	if strings.Contains(o.closeLead, "\n") {
		return "\n" + indentOf(o.closeLead) + detectIndentUnit(o.closeLead)
	}
	return ""
}

// indentOf returns the whitespace after the last newline in s.
func indentOf(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// detectIndentUnit guesses the file's indent unit from a line's leading
// whitespace: runs of spaces or tabs at the start of any line.
func detectIndentUnit(s string) string {
	for lines := strings.Split(s, "\n"); len(lines) > 0; lines = lines[1:] {
		line := lines[0]
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed != line && trimmed != "" {
			return line[:len(line)-len(trimmed)]
		}
	}
	return "  "
}

// Array is a JSON Array that remembers item order and raw slices.
type Array struct {
	entries   []entry
	closeLead string
	// strict marks arrays parsed by ParseStrict.
	strict bool
}

func (a *Array) render(b *strings.Builder) {
	b.WriteString("[")
	for i := range a.entries {
		a.entries[i].render(b)
	}
	b.WriteString(a.closeLead)
	b.WriteString("]")
}

func (a *Array) raw() string {
	var b strings.Builder
	a.render(&b)
	return b.String()
}

func (a *Array) hasComments() bool {
	if hasComment(a.closeLead) {
		return true
	}
	for i := range a.entries {
		e := &a.entries[i]
		if hasComment(e.lead) || hasComment(e.commaSep) {
			return true
		}
		if e.val.hasComments() {
			return true
		}
	}
	return false
}

// Len returns the number of items.
func (a *Array) Len() int { return len(a.entries) }

// StringItem returns item i as a decoded JSON string, with ok=false when it
// is not a string.
func (a *Array) StringItem(i int) (string, bool) {
	lit, ok := a.entries[i].val.(*literal)
	if !ok {
		return "", false
	}
	var s string
	if err := json.Unmarshal([]byte(lit.text), &s); err != nil {
		return "", false
	}
	return s, true
}

// ObjectItem returns item i as an Object, or nil when it is not one.
func (a *Array) ObjectItem(i int) *Object {
	sub, _ := a.entries[i].val.(*Object)
	return sub
}

// Append adds an item given as JSON text (compact) at the end, keeping a
// trailing-comma file style when the array had one.
func (a *Array) Append(rawValue string) {
	e := entry{lead: a.newItemLead(), val: &literal{text: rawValue, generated: true}}
	if n := len(a.entries); n > 0 {
		switch {
		case a.entries[n-1].commaSep == "":
			a.entries[n-1].commaSep = ","
		case strings.HasSuffix(a.entries[n-1].commaSep, ","):
			e.commaSep = "," // trailing-comma style
		}
	}
	a.entries = append(a.entries, e)
}

// Delete removes item i. Deleting the last item strips the previous item's
// comma when the document is strict.
func (a *Array) Delete(i int) {
	wasLast := i == len(a.entries)-1
	a.entries = append(a.entries[:i], a.entries[i+1:]...)
	if wasLast && a.strict {
		if n := len(a.entries); n > 0 {
			a.entries[n-1].commaSep = ""
		}
	}
}

// hasTrailingComma reports whether the last item carries a comma.
func (a *Array) hasTrailingComma() bool {
	if n := len(a.entries); n > 0 && a.entries[n-1].commaSep != "" {
		return true
	}
	for i := range a.entries {
		if a.entries[i].val.hasTrailingComma() {
			return true
		}
	}
	return false
}

// newItemLead mirrors newEntryLead for Array items.
func (a *Array) newItemLead() string {
	if n := len(a.entries); n > 0 {
		prev := a.entries[n-1].lead
		if strings.Contains(prev, "\n") {
			return "\n" + indentOf(prev)
		}
		return " "
	}
	if strings.Contains(a.closeLead, "\n") {
		return "\n" + indentOf(a.closeLead) + detectIndentUnit(a.closeLead)
	}
	return ""
}

// Quote encodes s as a JSON string literal. It is exported so the write
// sides can build generated values without hand-escaping skill names.
func Quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return string(b)
}

// quoteJSON encodes s as a JSON string literal.
func quoteJSON(s string) string { return Quote(s) }

type parser struct {
	src    string
	pos    int
	strict bool
}

func (p *parser) parseValue() (value, error) {
	p.skipSpaceAndComments()
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("unexpected end of input")
	}
	switch c := p.src[p.pos]; {
	case c == '{':
		return p.parseObject()
	case c == '[':
		return p.parseArray()
	case c == '"':
		start := p.pos
		if _, err := p.parseString(); err != nil {
			return nil, err
		}
		return &literal{text: p.src[start:p.pos]}, nil
	case strings.HasPrefix(p.src[p.pos:], "true"):
		p.pos += 4
		return &literal{text: "true"}, nil
	case strings.HasPrefix(p.src[p.pos:], "false"):
		p.pos += 5
		return &literal{text: "false"}, nil
	case strings.HasPrefix(p.src[p.pos:], "null"):
		p.pos += 4
		return &literal{text: "null"}, nil
	case c == '-' || (c >= '0' && c <= '9'):
		start := p.pos
		p.pos++
		for p.pos < len(p.src) && strings.ContainsRune("-+.eE0123456789", rune(p.src[p.pos])) {
			p.pos++
		}
		return &literal{text: p.src[start:p.pos]}, nil
	default:
		return nil, fmt.Errorf("unexpected character %q at offset %d", c, p.pos)
	}
}

func (p *parser) parseObject() (value, error) {
	open := p.pos
	p.pos++ // '{'
	o := &Object{strict: p.strict}
	for {
		leadStart := p.pos
		p.skipSpaceAndComments()
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unterminated Object at offset %d", open)
		}
		if p.src[p.pos] == '}' {
			o.closeLead = p.src[leadStart:p.pos]
			p.pos++
			return o, nil
		}

		var e entry
		e.lead = p.src[leadStart:p.pos]
		keyStart := p.pos
		key, err := p.parseString()
		if err != nil {
			return nil, fmt.Errorf("expected Object key at offset %d: %w", p.pos, err)
		}
		e.key, e.keyRaw = key, p.src[keyStart:p.pos]

		p.skipSpaceAndComments()
		if p.pos >= len(p.src) || p.src[p.pos] != ':' {
			return nil, fmt.Errorf("expected ':' after key %q at offset %d", key, p.pos)
		}
		valStart := p.pos
		p.pos++ // ':'
		p.skipSpaceAndComments()
		e.colon = p.src[valStart:p.pos]

		e.val, err = p.parseValue()
		if err != nil {
			return nil, err
		}

		if err := p.entryTail(&e); err != nil {
			return nil, err
		}
		o.entries = append(o.entries, e)
	}
}

// entryTail records the text from the value's end through a following
// comma (entry.commaSep). Without a comma the position is restored so the
// whitespace before the closing bracket stays in the container's closeLead.
func (p *parser) entryTail(e *entry) error {
	afterVal := p.pos
	p.skipSpaceAndComments()
	if p.pos < len(p.src) && p.src[p.pos] == ',' {
		p.pos++
		e.commaSep = p.src[afterVal:p.pos]
		return nil
	}
	if p.pos < len(p.src) && p.src[p.pos] != '}' && p.src[p.pos] != ']' {
		return fmt.Errorf("expected ',' at offset %d", p.pos)
	}
	p.pos = afterVal
	return nil
}

func (p *parser) parseArray() (value, error) {
	open := p.pos
	p.pos++ // '['
	a := &Array{strict: p.strict}
	for {
		leadStart := p.pos
		p.skipSpaceAndComments()
		if p.pos >= len(p.src) {
			return nil, fmt.Errorf("unterminated Array at offset %d", open)
		}
		if p.src[p.pos] == ']' {
			a.closeLead = p.src[leadStart:p.pos]
			p.pos++
			return a, nil
		}

		var e entry
		e.lead = p.src[leadStart:p.pos]
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		e.val = val

		if err := p.entryTail(&e); err != nil {
			return nil, err
		}
		a.entries = append(a.entries, e)
	}
}

// parseString consumes a JSON string literal and returns its decoded value.
func (p *parser) parseString() (string, error) {
	start := p.pos
	p.pos++ // opening quote
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '\\':
			p.pos += 2
		case '"':
			p.pos++
			var decoded string
			if err := json.Unmarshal([]byte(p.src[start:p.pos]), &decoded); err != nil {
				return "", fmt.Errorf("invalid string escape at offset %d", start)
			}
			return decoded, nil
		default:
			p.pos++
		}
	}
	return "", fmt.Errorf("unterminated string at offset %d", start)
}

// skipSpaceAndComments advances past whitespace and comments, leaving pos
// on the next meaningful byte.
func (p *parser) skipSpaceAndComments() {
	for p.pos < len(p.src) {
		switch c := p.src[p.pos]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.pos++
		case c == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '/':
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		case c == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '*':
			p.pos += 2
			for p.pos+1 < len(p.src) && (p.src[p.pos] != '*' || p.src[p.pos+1] != '/') {
				p.pos++
			}
			if p.pos+1 < len(p.src) {
				p.pos += 2
			} else {
				p.pos = len(p.src)
			}
		default:
			return
		}
	}
}
