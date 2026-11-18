import { config } from "dotenv";
import { fileURLToPath } from "node:url";
import { defineConfig, env } from "prisma/config";

// Share host configuration while keeping this module's migrations independent.
// Existing process variables take precedence over the root .env.
config({ path: fileURLToPath(new URL("../../.env", import.meta.url)), quiet: true });

export default defineConfig({
  schema: "../../prisma/athub/schema.prisma",
  migrations: { path: "../../prisma/athub/migrations" },
  datasource: { url: env("ATHUB_DATABASE_URL") },
});
