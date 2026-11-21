import Image from "next/image";

export function PayGuardLogo() {
  return (
    <span className="relative block h-9 w-[122px] shrink-0 overflow-hidden">
      <Image
        src="/payguard-logo.png"
        alt="PayGuard"
        width={577}
        height={433}
        unoptimized
        className="absolute top-1/2 left-1/2 h-auto w-[138px] max-w-none -translate-x-1/2 -translate-y-1/2"
      />
    </span>
  );
}
