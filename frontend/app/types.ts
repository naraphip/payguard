export type Product = "flight_ticket" | "hotel_room" | "physical" | "digital";
export type SimulationProduct = Product | "payment_gateway";
export type Scenario = "normal" | "fraud" | "mixed";
export type EntityKind = "account" | "device" | "ip" | "token";

export type Config = {
  seed: number;
  product: SimulationProduct;
  scenario: Scenario;
  accounts: number;
  rate: number;
  quantity: number;
  date: string;
  delivery: string;
  rotateIp: boolean;
  sharedIp: boolean;
  sharedDevice: boolean;
  sharedToken: boolean;
  newAccount: boolean;
  retry: boolean;
  missingToken: boolean;
  jevTimeout: boolean;
  merchantPolicy: boolean;
  contactEmailDomain?: string;
};

export type Entity = { kind: EntityKind; key: string };

export type Decision = {
  event: {
    id: string;
    orderId: string;
    stage: string;
    at: number;
    product: SimulationProduct;
    quantity: number;
    domain: string;
    ageDays: number;
    entities: Entity[];
  };
  action: string;
  actionSource: string;
  platformDecision: string;
  merchantPolicyDecision: string;
  ruleHits: string[];
  jevStatus: string;
  jevAnswer: string;
  missing: string[];
  features: { kind: EntityKind; orders: number }[];
};

export type Run = {
  id: string;
  config: Config;
  generatorVersion: string;
  ruleVersion: string;
  jevVersion: string;
  duplicates: number;
  decisions: Decision[];
};

export type Report = {
  tp: number;
  fp: number;
  tn: number;
  fn: number;
  unscored: number;
  samples: number;
  precision: number | null;
  recall: number | null;
  fpr: number | null;
};
