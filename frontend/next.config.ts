import type { NextConfig } from "next";

const config: NextConfig = {
  devIndicators: false,
  async rewrites() {
    const api = process.env.PAYGUARD_API_URL ?? "http://127.0.0.1:8081";
    return [
      { source: "/api/v1/simulation/:path*", destination: `${api}/api/v1/simulation/:path*` },
    ];
  },
};

export default config;
