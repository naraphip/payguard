import { ImageResponse } from "next/og";

export const size = { width: 32, height: 32 };
export const contentType = "image/png";

export default function Icon() {
  return new ImageResponse(
    <div style={{
      display: "flex", width: "100%", height: "100%", alignItems: "center",
      justifyContent: "center", borderRadius: 7, background: "#17243a",
      color: "white", fontSize: 14, fontWeight: 700,
    }}>PG</div>,
    size,
  );
}
