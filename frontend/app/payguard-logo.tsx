import Image from "next/image";

export function PayGuardLogo({ compact = false }: { compact?: boolean }) {
  return (
    <Image
      src={compact ? "/brand/payguard-symbol.svg?v=4" : "/brand/payguard-logo-horizontal.svg?v=4"}
      alt=""
      width={compact ? 280 : 954}
      height={compact ? 265 : 266}
      priority
      unoptimized
      className={compact ? "h-auto w-9 shrink-0" : "h-auto w-[190px] shrink-0"}
    />
  );
}
