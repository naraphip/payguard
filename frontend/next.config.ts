import type { NextConfig } from "next";

const config: NextConfig = {
  devIndicators: false,
  async rewrites() {
    const configuredApi = process.env.PAYGUARD_API_URL ?? "http://127.0.0.1:8081";
    const api = /^https?:\/\//i.test(configuredApi) ? configuredApi : `https://${configuredApi}`;
    return [
      { source: "/api/v1/simulation/:path*", destination: `${api}/api/v1/simulation/:path*` },
      { source: "/api/v1/system/rules/:path*", destination: `${api}/api/v1/system/rules/:path*` },
      { source: "/api/v1/booking/:path*", destination: `${api}/api/v1/booking/:path*` },
    ];
  },
};

export default config;
