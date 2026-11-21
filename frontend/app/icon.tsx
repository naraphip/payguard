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
      <svg width="29" height="29" viewBox="0 0 256 256" fill="none">
        <defs>
          <linearGradient id="blue" x2="1" y2="1">
            <stop stopColor="#16A6FF" />
            <stop offset="1" stopColor="#0753D9" />
          </linearGradient>
          <linearGradient id="mint" x2="1" y2="1">
            <stop stopColor="#55F0C5" />
            <stop offset="1" stopColor="#00B887" />
          </linearGradient>
        </defs>
        <rect width="256" height="256" rx="56" fill="#FFFFFF" />
        <g transform="translate(18 22)">
          <circle cx="91" cy="44" r="25" fill="url(#blue)" />
          <path d="M38 83C39 60 59 61 75 74C94 88 108 74 126 64C147 53 159 68 147 80C130 92 111 99 96 120L112 169C73 163 33 137 28 110C26 98 29 90 38 83Z" fill="url(#blue)" />
          <path d="M100 116L153 93L192 109L190 147C180 180 153 192 153 192C121 177 109 150 100 116Z" fill="url(#mint)" stroke="white" strokeWidth="8" strokeLinejoin="round" />
          <path d="M131 143L148 159L175 128" fill="none" stroke="white" strokeWidth="10" strokeLinecap="round" strokeLinejoin="round" />
          <path d="M165 25V39M183 32L175 43" stroke="#00C389" strokeWidth="9" strokeLinecap="round" />
        </g>
      </svg>
    </div>,
    size,
  );
}
