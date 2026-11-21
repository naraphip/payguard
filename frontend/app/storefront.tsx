import Image from "next/image";
import { CalendarDays, Check, Clock3, Gift, Headphones, Hotel, Package, Plane, ShoppingBag } from "lucide-react";
import type { Product } from "./types";

export const catalog = {
  flight_ticket: {
    title: "Bangkok to Singapore", shortTitle: "Bangkok → Singapore", type: "Flight",
    detail: "BKK → SIN · Nonstop · Economy", extra: "2h 25m", unit: "traveler", quantity: "Travelers",
    price: 459000, image: "/storefront/flight.webp", icon: Plane,
  },
  hotel_room: {
    title: "Marina Bay City Hotel", shortTitle: "Marina Bay City Hotel", type: "Hotel",
    detail: "Singapore · Deluxe king room", extra: "1 night", unit: "room", quantity: "Rooms",
    price: 325000, image: "/storefront/hotel.webp", icon: Hotel,
  },
  physical: {
    title: "Studio Wireless Headphones", shortTitle: "Wireless headphones", type: "Electronics",
    detail: "Over-ear · Noise cancellation", extra: "Ships in 2–4 days", unit: "pair", quantity: "Quantity",
    price: 249000, image: "/storefront/headphones.webp", icon: Headphones,
  },
  digital: {
    title: "Digital Gift Card", shortTitle: "Digital gift card", type: "Digital goods",
    detail: "Store credit · No physical delivery", extra: "Instant email delivery", unit: "card", quantity: "Quantity",
    price: 75000, image: "/storefront/gift.webp", icon: Gift,
  },
} as const;

export type Prices = Record<Product, number>;

export const defaultPrices: Prices = {
  flight_ticket: catalog.flight_ticket.price,
  hotel_room: catalog.hotel_room.price,
  physical: catalog.physical.price,
  digital: catalog.digital.price,
};

const baht = new Intl.NumberFormat("en-TH", {
  style: "currency",
  currency: "THB",
  minimumFractionDigits: 0,
  maximumFractionDigits: 2,
});

export function thb(satang: number) {
  return baht.format(satang / 100);
}

export function travelProduct(product: Product) {
  return product === "flight_ticket" || product === "hotel_room";
}

export function Storefront({ selected, prices, choose }: {
  selected: Product;
  prices: Prices;
  choose: (product: Product) => void;
}) {
  const travel = travelProduct(selected);
  const products: Product[] = travel ? ["flight_ticket", "hotel_room"] : ["physical", "digital"];
  return <section aria-labelledby="storefront-title" className="space-y-3">
    <h2 id="storefront-title" className="sr-only">Storefront</h2>
    <div className="flex flex-wrap gap-2" role="group" aria-label="Storefront">
      <button type="button" aria-pressed={travel} className="shop-pill" onClick={() => { if (!travel) choose("flight_ticket"); }}>
        <Plane size={16} aria-hidden="true" />Travel
      </button>
      <button type="button" aria-pressed={!travel} className="shop-pill" onClick={() => { if (travel) choose("physical"); }}>
        <ShoppingBag size={16} aria-hidden="true" />Online store
      </button>
    </div>
    <div className="grid gap-3 sm:grid-cols-2 lg:gap-4" role="group" aria-label="Choose a product">
      {products.map(key => {
        const product = catalog[key];
        const active = key === selected;
        const Icon = product.icon;
        return <article key={key} className="shop-product" data-selected={active}>
          <div className="relative min-h-[122px] overflow-hidden bg-[#e8def3] sm:aspect-[16/8]">
            <Image src={product.image} alt="" fill sizes="(max-width: 639px) 33vw, (max-width: 1023px) 46vw, 32vw"
              className={key === "flight_ticket" ? "object-cover object-[68%_center]" : "object-cover"} />
            <span className="absolute top-2 left-2 flex items-center gap-1 rounded-full bg-white px-2 py-1 text-[11px] font-semibold text-[#31223f] shadow-sm sm:top-3 sm:left-3 sm:gap-1.5 sm:px-2.5 sm:text-xs">
              <Icon size={13} className="hidden sm:block" aria-hidden="true" />{product.type}
            </span>
          </div>
          <div className="min-w-0 space-y-2 p-3 sm:space-y-3 sm:p-[17px]">
            <div>
              <h3 className="text-[15px] font-semibold leading-snug tracking-tight text-[#2d203c] sm:text-[17px]">{product.title}</h3>
              <p className="mt-1 text-xs leading-relaxed text-[#5f516d] sm:text-[13px]">{product.detail}</p>
              <p className="mt-1.5 flex items-center gap-1.5 text-xs font-medium text-[#5f516d]">
                {travel ? <Clock3 size={13} aria-hidden="true" /> : <Package size={13} aria-hidden="true" />}{product.extra}
              </p>
            </div>
            <div className="flex min-w-0 flex-wrap items-center justify-between gap-x-2 gap-y-1 border-t border-[#eee7f4] pt-2 sm:pt-3">
              <p className="text-sm font-semibold text-[#352044] sm:text-base">{thb(prices[key])} <span className="text-xs font-normal text-[#5f516d]">/ {product.unit}</span></p>
              <span className={`inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-1 text-[11px] font-semibold sm:px-2.5 sm:text-xs ${active ? "bg-[#eee1fa] text-[#5b2783]" : "bg-[#f6f1fa] text-[#4c355f]"}`}>
                {active && <Check size={13} aria-hidden="true" />}{active ? "Selected" : "Select"}
              </span>
            </div>
          </div>
          <button type="button" aria-pressed={active} className="shop-product-select"
            aria-label={`Select ${product.title}, ${thb(prices[key])} per ${product.unit}`}
            onClick={() => choose(key)} />
        </article>;
      })}
    </div>
  </section>;
}

export function CartProduct({ product, date, delivery }: { product: Product; date: string; delivery: string }) {
  const item = catalog[product];
  const Icon = travelProduct(product) ? CalendarDays : Package;
  const dateValue = new Date(`${date}T00:00:00Z`);
  const bookingDate = Number.isFinite(dateValue.getTime())
    ? new Intl.DateTimeFormat("en-GB", { dateStyle: "long", timeZone: "UTC" }).format(dateValue)
    : "";
  const detail = travelProduct(product)
    ? bookingDate || "Choose a booking date"
    : delivery === "instant" ? "Instant email delivery" : delivery === "express" ? "Express shipping" : "Standard shipping";

  return <div className="flex min-w-0 items-center gap-3">
    <div className="relative h-[58px] w-[66px] shrink-0 overflow-hidden rounded-xl bg-[#eae0f5]">
      <Image src={item.image} alt="" fill sizes="66px" className={product === "flight_ticket" ? "object-cover object-[68%_center]" : "object-cover"} />
    </div>
    <div className="min-w-0">
      <p className="text-sm font-semibold text-[#31223f]">{item.shortTitle}</p>
      <p className="mt-1 flex items-center gap-1.5 text-xs text-[#5f516d]"><Icon size={13} className="shrink-0" aria-hidden="true" />{detail}</p>
    </div>
  </div>;
}
