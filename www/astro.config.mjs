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
          label: "Start here",
          items: [{ label: "Installation", slug: "installation" }],
        },
        {
          label: "Reference",
          items: [
            { label: "Command reference", slug: "cli" },
            { label: "Per-harness reference", slug: "harnesses" },
            { label: "State file schema", slug: "state-file" },
            { label: "Undo & escape hatches", slug: "undo" },
          ],
        },
        {
          label: "Skills",
          items: [{ label: "Skills catalog", slug: "skills" }],
        },
      ],
      customCss: ["./src/styles/home.css"],
      head: [
        {
          tag: "meta",
          attrs: {
            property: "og:image",
            content: "https://fleet.zzacong.com/og-image.png",
          },
        },
        {
          tag: "meta",
          attrs: { property: "og:image:width", content: "1200" },
        },
        {
          tag: "meta",
          attrs: { property: "og:image:height", content: "630" },
        },
        {
          tag: "meta",
          attrs: {
            property: "og:image:alt",
            content: "Fleet — manage agent skills across AI coding agents",
          },
        },
        {
          tag: "meta",
          attrs: {
            name: "twitter:image",
            content: "https://fleet.zzacong.com/og-image.png",
          },
        },
      ],
    }),
  ],
});
