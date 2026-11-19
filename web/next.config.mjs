/** @type {import('next').NextConfig} */
const backend = process.env.OAUTH_BACKEND_URL || "http://localhost:8010"
const imageBackend = process.env.IMAGE_BACKEND_URL || backend
const creatorBackend = process.env.CREATPLAT_BACKEND_URL || "http://localhost:8010"
// Documentation lives in the standalone Wiki service; historic in-app addresses redirect there.
const wiki = (process.env.NEXT_PUBLIC_WIKI_URL || "http://localhost:3200").replace(/\/+$/, "")

const nextConfig = {
  // The development badge otherwise covers the bottom-left account entry.
  devIndicators: false,
  transpilePackages: ["shiki"],
  distDir: process.env.NEXT_OUTPUT_DIR || ".next",
  async redirects() {
    return [
      { source: "/admin/integration", destination: `${wiki}/integration`, permanent: false },
      { source: "/account/integration", destination: `${wiki}/integration`, permanent: false },
      { source: "/wiki", destination: wiki, permanent: false },
      { source: "/wiki/:path*", destination: `${wiki}/:path*`, permanent: false },
    ]
  },
  async headers() {
    return ["/consent", "/pay/:path*", "/account/:userId/wallet", "/account/wallet", "/account/:userId/security", "/account/security", "/account/:userId/key", "/account/key", "/admin/users", "/admin/imgbed"].map(source => ({ source, headers: [
      { key: "X-Frame-Options", value: "DENY" },
      { key: "Content-Security-Policy", value: "frame-ancestors 'none'" },
      { key: "Referrer-Policy", value: "no-referrer" },
      { key: "Cache-Control", value: "no-store" },
    ] }))
  },
  async rewrites() {
    return [
      { source: "/api/v1/:path*", destination: `${backend}/api/v1/:path*` },
      { source: "/api/v2/:path*", destination: `${imageBackend}/api/v2/:path*` },
      // Mail uses the account session, so it shares the account backend address.
      { source: "/api/v3/:path*", destination: `${backend}/api/v3/:path*` },
      { source: "/api/v4/:path*", destination: `${creatorBackend}/api/v4/:path*` },
      { source: "/oauth/:path*", destination: `${backend}/oauth/:path*` },
      { source: "/.well-known/:path*", destination: `${backend}/.well-known/:path*` },
    ]
  },
}

export default nextConfig
