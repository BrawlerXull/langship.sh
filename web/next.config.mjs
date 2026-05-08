/** @type {import('next').NextConfig} */
const isProd = process.env.NODE_ENV === "production";

// Where the Go backend is listening during `npm run dev`.
const apiTarget = process.env.FLOW_API_URL ?? "http://localhost:8090";

const nextConfig = {
  // Static export only at build time — the Go binary embeds ./out.
  // In dev, leave Next as a normal server so we can proxy /api → Go.
  ...(isProd ? { output: "export" } : {}),
  images: { unoptimized: true },
  trailingSlash: true,
  reactStrictMode: true,

  // Dev-only: forward API calls to the Go server.
  async rewrites() {
    if (isProd) return [];
    return [{ source: "/api/:path*", destination: `${apiTarget}/api/:path*` }];
  },
};

export default nextConfig;
