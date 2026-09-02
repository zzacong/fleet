import { defineConfig } from "astro/config";
import starlight from "@astrojs/starlight";

// https://starlight.astro.build/reference/configuration/
export default defineConfig({
  site: "https://fleet.example.com",
  integrations: [
    starlight({
      title: "Fleet",
      description: "Fleet manages agent skills across AI coding agents.",
      social: [
        { icon: "github", label: "GitHub", href: "https://github.com/zzacong/fleet" },
      ],
      sidebar: [
        {
          label: "Start Here",
          items: [
            { label: "Introduction", link: "/" },
          ],
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
          items: [
            { label: "Skills Catalog", slug: "skills" },
          ],
        },
      ],
      customCss: [],
    }),
  ],
});
