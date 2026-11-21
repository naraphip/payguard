import Image from "next/image";

export function PayGuardLogo() {
  return (
    <Image
      src="/brand/payguard-logo-primary.svg"
      alt="PayGuard"
      width={840}
      height={215}
      priority
      unoptimized
      className="h-auto w-[190px] shrink-0"
    />
  );
}
