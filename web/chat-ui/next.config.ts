import type { NextConfig } from "next";

const goBackend = process.env.MOVIEAGENT_API_ORIGIN || "http://127.0.0.1:8090";
const staticExport = process.env.NEXT_EXPORT === "1";

const nextConfig: NextConfig = {
  images: { unoptimized: true },
  // Dev: browser talks to Go directly (SSE survives >30s). Export: same-origin.
  env: {
    NEXT_PUBLIC_MOVIEAGENT_API_ORIGIN: staticExport
      ? ""
      : process.env.NEXT_PUBLIC_MOVIEAGENT_API_ORIGIN || goBackend,
  },
  ...(staticExport
    ? {
        output: "export" as const,
        trailingSlash: true,
      }
    : {
        async rewrites() {
          return [
            { source: "/api/v1/:path*", destination: `${goBackend}/api/v1/:path*` },
            { source: "/media/:path*", destination: `${goBackend}/media/:path*` },
          ];
        },
      }),
};

export default nextConfig;
