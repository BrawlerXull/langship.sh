/** @type {import('next').NextConfig} */
const nextConfig = {
  output: "export",
  // Static export writes to ./out (Go embed reads from there).
  // Leave distDir at the default ".next" so build artifacts and the
  // export target don't collide.
  images: { unoptimized: true },
  trailingSlash: true,
  reactStrictMode: true,
};

export default nextConfig;
