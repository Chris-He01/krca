/** @type {import('next').NextConfig} */
const isDev = process.env.NODE_ENV === 'development';

const nextConfig = {
  // Static export only for production build (served by Go backend).
  // Dev mode skips export so that rewrites/proxy work.
  ...(isDev ? {} : { output: 'export', distDir: 'out' }),
  trailingSlash: false,
  images: {
    unoptimized: true,
  },
  assetPrefix: '',
  // In dev, proxy all backend routes to the Go server on :8080
  ...(isDev && {
    async rewrites() {
      const backendBase = process.env.BACKEND_URL ?? 'http://localhost:8080';
      return [
        { source: '/v1/:path*',      destination: `${backendBase}/v1/:path*` },
        { source: '/healthz',        destination: `${backendBase}/healthz` },
        { source: '/api/healthz',    destination: `${backendBase}/api/healthz` },
      ];
    },
  }),
};

export default nextConfig;
