import { ImageResponse } from "next/og";

export const size = { width: 32, height: 32 };
export const contentType = "image/png";

export default function Icon() {
  return new ImageResponse(
    <div
      style={{
        display: "flex",
        width: "100%",
        height: "100%",
        alignItems: "center",
        justifyContent: "center",
        borderRadius: 7,
        background: "#FFFFFF",
      }}
    >
      <svg width="29" height="29" viewBox="0 0 36 36" fill="none">
        <defs>
          <linearGradient id="shield" x1="4" y1="3" x2="35" y2="33" gradientUnits="userSpaceOnUse">
            <stop stopColor="#0078FF" />
            <stop offset="1" stopColor="#00C389" />
          </linearGradient>
        </defs>
        <circle cx="13" cy="8" r="5" fill="#0078FF" />
        <path
          d="M4 17C10 12 18.5 13 24 17C27 19 29 17 32 15"
          stroke="url(#shield)"
          strokeWidth="6"
          strokeLinecap="round"
        />
        <path d="M7 20C12 18 17 21 19.5 26C21.5 30 24 31.5 27 33C20 34.5 13 31 9 26C7 24 6 21 7 20Z" fill="#0078FF" />
        <path d="M19 20L27 17L32 20V26C32 30 29 33 26 34C22 32.5 19 29.5 19 26V20Z" fill="#00C389" />
        <path d="M23 26L26 29L30 24" stroke="white" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    </div>,
    size,
  );
}
