/** @type {import('next').NextConfig} */
const backend = process.env.OAUTH_BACKEND_URL || "http://localhost:8080"

const nextConfig = {
  distDir: process.env.NEXT_OUTPUT_DIR || ".next",
  async redirects() {
    return [
      { source: "/admin/integration", destination: "/wiki/integration", permanent: false },
      { source: "/account/integration", destination: "/wiki/integration", permanent: false },
    ]
  },
  async headers() {
    return ["/consent", "/pay/:path*", "/account/:userId/wallet", "/account/wallet", "/account/:userId/security", "/account/security", "/admin/users", "/admin/notifications", "/admin/announcements", "/admin/emails"].map(source => ({ source, headers: [
      { key: "X-Frame-Options", value: "DENY" },
      { key: "Content-Security-Policy", value: "frame-ancestors 'none'" },
      { key: "Referrer-Policy", value: "no-referrer" },
      { key: "Cache-Control", value: "no-store" },
    ] }))
  },
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${backend}/api/:path*` },
      { source: "/oauth/:path*", destination: `${backend}/oauth/:path*` },
      { source: "/.well-known/:path*", destination: `${backend}/.well-known/:path*` },
    ]
  },
}

export default nextConfig
