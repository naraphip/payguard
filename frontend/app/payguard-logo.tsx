export function PayGuardLogo() {
  return (
    <span
      className="block h-9 w-[176px] shrink-0"
      role="img"
      aria-label="PayGuard"
    >
      <svg
        viewBox="0 0 176 36"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="h-full w-full"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id="payguard-icon" x1="4" y1="3" x2="35" y2="33" gradientUnits="userSpaceOnUse">
            <stop stopColor="#0078FF" />
            <stop offset="1" stopColor="#00C389" />
          </linearGradient>
        </defs>

        <circle cx="16" cy="9" r="6" fill="#0078FF" />
        <path
          d="M5 17.5C11.5 11 21.5 12.5 27 16.5C30 18.3 32 16.8 34.5 15.5"
          stroke="url(#payguard-icon)"
          strokeWidth="7"
          strokeLinecap="round"
        />
        <path
          d="M7 20.5C12.5 18 18 20.5 20.5 25.5C22.5 29.5 24.5 31 27.5 32.5C20.5 34 13.5 30.5 9.5 25.5C7.5 23 6.5 21.5 7 20.5Z"
          fill="#0078FF"
        />
        <path
          d="M20.5 20.5L29.5 17.5L35 20.5V26C35 30.5 31.5 33.5 27.5 35C23.5 33.5 20 30.5 20 26V20.5Z"
          fill="#00C389"
        />
        <path
          d="M24.5 26L27.5 29L32.5 23.5"
          stroke="white"
          strokeWidth="2.4"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path d="M38 9V13M42 7L40.5 10M34 6L35.5 9" stroke="#00C389" strokeWidth="2.8" strokeLinecap="round" />

        <text
          x="48"
          y="24"
          fill="#0A2E6E"
          fontFamily="Arial, sans-serif"
          fontSize="27"
          fontWeight="800"
          letterSpacing="-1.6"
        >
          Pay
        </text>
        <text
          x="91"
          y="24"
          fill="#00C389"
          fontFamily="Arial, sans-serif"
          fontSize="27"
          fontWeight="800"
          letterSpacing="-1.6"
        >
          Guard
        </text>
        <text
          x="50"
          y="33"
          fill="#17243A"
          fontFamily="Arial, sans-serif"
          fontSize="8.5"
          fontWeight="700"
          letterSpacing="0.2"
        >
          Safer Payments. Happier You.
        </text>
      </svg>
    </span>
  );
}
