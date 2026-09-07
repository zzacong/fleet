import starlight from "@astrojs/starlight";
import { defineConfig } from "astro/config";

// https://astro.build/config
// https://starlight.astro.build/reference/configuration/
export default defineConfig({
  site: "https://fleet.zzacong.com",
  integrations: [
    starlight({
      title: "Fleet",
      description: "Fleet manages agent skills across AI coding agents.",
      logo: {
        light: "./src/assets/fleet-logo-light.svg",
        dark: "./src/assets/fleet-logo-dark.svg",
        alt: "Fleet",
      },
      social: [
        {
          icon: "github",
          label: "GitHub",
          href: "https://github.com/zzacong/fleet",
        },
      ],
      sidebar: [
        {
          label: "Start Here",
          items: [{ label: "Installation", slug: "installation" }],
        },
        {
          label: "Reference",
          items: [
            { label: "Command Reference", slug: "cli" },
            { label: "Per-Harness Reference", slug: "harnesses" },
            { label: "State File Schema", slug: "state-file" },
            { label: "Undo & Escape Hatches", slug: "undo" },
          ],
        },
        {
          label: "Skills",
          items: [{ label: "Skills Catalog", slug: "skills" }],
        },
      ],
      customCss: [],
      // Downgraded from Starlight's summary_large_image default until the
      // logo pass adds a real og:image.
      head: [
        {
          tag: "meta",
          attrs: { name: "twitter:card", content: "summary" },
        },
      ],
    }),
  ],
});
