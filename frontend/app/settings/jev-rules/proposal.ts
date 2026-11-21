const signalLabels = {
  amountMinor: "Booking amount",
  bookingVelocity60s: "Booking velocity",
};

const comparisons = {
  "at least": "gte",
  "greater than": "gt",
  "at most": "lte",
  "less than": "lt",
  "equal to": "eq",
} as const;

type Signal = keyof typeof signalLabels;
type Operator = (typeof comparisons)[keyof typeof comparisons];
type Condition = { signal: Signal; operator: Operator; value: number };
export type Proposal = { name: string; action: "REVIEW"; conditions: Condition[] };

export const examplePrompts = [
  "Review bookings when amount is at least 50000 minor units",
  "Review bookings when velocity is greater than 3 bookings in 60 seconds",
];

const formatError = "Use an example prompt or paste rule JSON. Specify the comparison, a whole number, and the units for every condition.";

function hasKeys(value: unknown, keys: string[]): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    && Object.keys(value).length === keys.length && keys.every((key) => Object.hasOwn(value, key));
}

function validate(value: unknown): Proposal {
  if (!hasKeys(value, ["name", "action", "conditions"]) || typeof value.name !== "string"
    || !value.name.trim() || [...value.name.trim()].length > 80 || /[\u0000-\u001F\u007F-\u009F]/.test(value.name.trim())
    || value.action !== "REVIEW"
    || !Array.isArray(value.conditions) || value.conditions.length < 1 || value.conditions.length > 5) {
    throw new Error("Use a name of 1–80 characters without line breaks or control characters, the REVIEW action, and 1–5 conditions.");
  }
  const conditions = value.conditions.map((condition: unknown): Condition => {
    if (!hasKeys(condition, ["signal", "operator", "value"])
      || typeof condition.signal !== "string" || !Object.hasOwn(signalLabels, condition.signal)
      || !Object.values(comparisons).includes(condition.operator as Operator)
      || typeof condition.value !== "number" || !Number.isSafeInteger(condition.value) || condition.value < 0) {
      throw new Error("Use amountMinor or bookingVelocity60s, an operator of gte, gt, lte, lt, or eq, and a nonnegative safe integer.");
    }
    return { signal: condition.signal as Signal, operator: condition.operator as Operator, value: condition.value };
  });
  return { name: value.name.trim(), action: "REVIEW", conditions };
}

export function parseProposal(input: string): Proposal {
  const text = input.trim();
  const fenced = /^\x60\x60\x60(?:json)?\s*([\s\S]+?)\s*\x60\x60\x60$/i.exec(text);
  if (fenced || text.startsWith("{")) {
    let json: unknown;
    try {
      json = JSON.parse(fenced?.[1] ?? text);
    } catch {
      throw new Error("The rule JSON is invalid.");
    }
    return validate(json);
  }

  const prompt = /^review bookings when (.+?)\.?$/i.exec(text.replace(/\s+/g, " "));
  if (!prompt) throw new Error(formatError);
  const clauses = prompt[1].toLowerCase().split(" and ");
  const conditions = clauses.map((clause) => {
    const match = /^(amount|velocity) is (at least|greater than|at most|less than|equal to) (\d+) (minor units|bookings in 60 seconds)$/.exec(clause);
    if (!match || (match[1] === "amount") !== (match[4] === "minor units")) throw new Error(formatError);
    return {
      signal: match[1] === "amount" ? "amountMinor" : "bookingVelocity60s",
      operator: comparisons[match[2] as keyof typeof comparisons],
      value: Number(match[3]),
    };
  });
  const name = conditions.length === 1 ? `${signalLabels[conditions[0].signal as Signal]} review` : "Combined booking review";
  return validate({ name, action: "REVIEW", conditions });
}
