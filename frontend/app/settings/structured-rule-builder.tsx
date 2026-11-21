"use client";

import { useRef, useState } from "react";
import type { FormEvent } from "react";
import { merchantPolicyProducts, policyActionLabels } from "../merchant-demo";
import type { BusinessProfile, MerchantProposal, PolicyAction, PolicyField } from "./rule-composer";
import { outlineButton, primaryButton, stages } from "./system-rules/contracts";

const productLabels: Record<string, string> = merchantPolicyProducts;
const actions: PolicyAction[] = ["BLOCK", "REVIEW", "CHALLENGE"];
export const merchantFieldLabels: Record<string, string> = {
  "product.type": "Product",
  "customer.email_domain": "Customer email domain",
  "customer.mailbox_ref": "Exact email address",
  "order.amount_minor": "Order amount in minor units",
  "transaction.description": "Transaction description",
};
export const merchantOperatorLabels: Record<string, string> = {
  eq: "equals", ne: "does not equal", gt: "is greater than", gte: "is at least",
  lt: "is less than", lte: "is at most", contains: "contains", semantic_match: "matches the meaning of",
};

type DraftCondition = { id: number; field: string; operator: string; value: string };

const stageForProduct = (product: string) => product === "flight_ticket" || product === "hotel_room" ? "BOOKING" : "CHECKOUT";

export function policyFieldLabel(field: PolicyField) {
  return field.label ?? merchantFieldLabels[field.fieldName] ?? field.fieldName;
}

function conditionValue(field: PolicyField, value: string) {
  const input = value.trim();
  const label = policyFieldLabel(field);
  if (!input) throw new Error(`Enter a value for ${label}.`);
  switch (field.dataType) {
    case "boolean":
      if (input !== "true" && input !== "false") throw new Error(`Choose Yes or No for ${label}.`);
      return input === "true";
    case "number": {
      const number = Number(input);
      if (!/^-?\d+$/.test(input) || !Number.isSafeInteger(number) || (field.fieldName === "order.amount_minor" && number < 0)) {
        throw new Error(`Enter a valid whole number for ${label}.`);
      }
      return number;
    }
    case "country":
      if (!/^[A-Za-z]{2}$/.test(input)) throw new Error(`Enter a two-letter country code for ${label}.`);
      return input.toUpperCase();
    default:
      return input;
  }
}

function ConditionValue({ field, value, products, semantic, onChange }: {
  field?: PolicyField;
  value: string;
  products: string[];
  semantic: boolean;
  onChange: (value: string) => void;
}) {
  return <label className="block min-w-0 text-sm font-medium">{semantic ? "Meaning to match" : "Value"}
    {field?.dataType === "boolean" || field?.dataType === "enum" ? <select className="control mt-1.5" value={value}
      onChange={event => onChange(event.currentTarget.value)}>
      <option value="">Select value</option>
      {field.dataType === "boolean"
        ? <><option value="true">Yes</option><option value="false">No</option></>
        : products.map(product => <option key={product} value={product}>{productLabels[product] ?? product}</option>)}
    </select> : <input className="control mt-1.5" disabled={!field} value={value} autoComplete="off"
      maxLength={field?.dataType === "country" ? 2 : field?.dataType === "text" ? 120 : 253}
      inputMode={field?.dataType === "number" ? "numeric" : "text"}
      placeholder={semantic ? "e.g. a Hong Kong Disneyland admission ticket" : field?.dataType === "country" ? "e.g. IN" : field?.fieldName === "customer.email_domain" ? "example.test" : undefined}
      onChange={event => onChange(event.currentTarget.value)} />}
  </label>;
}

export function StructuredRuleBuilder({ profile, initial, semanticField, disabled, onChange, onPreview }: {
  profile: BusinessProfile;
  initial?: MerchantProposal;
  semanticField?: string;
  disabled: boolean;
  onChange: () => void;
  onPreview: (proposal: MerchantProposal) => void;
}) {
  const [name, setName] = useState(initial?.name ?? "");
  const [products, setProducts] = useState(initial?.product_types ?? (!semanticField && profile.products.length === 1 ? profile.products.slice() : []));
  const [action, setAction] = useState<PolicyAction | "">(initial?.action ?? (semanticField ? "" : "REVIEW"));
  const [logic, setLogic] = useState<"AND" | "OR">(initial?.conditions.logic ?? "AND");
  const [conditions, setConditions] = useState<DraftCondition[]>(() => initial?.conditions.rules.map((condition, id) => ({
    ...condition, id, value: String(condition.value),
  })) ?? [{ id: 0, field: semanticField ?? "", operator: semanticField ? "semantic_match" : "", value: "" }]);
  const nextId = useRef(conditions.length);
  const [error, setError] = useState("");
  const fieldOptions = (profile.fieldRegistry ?? []).filter(field => field.fieldName !== "customer.mailbox_ref" &&
    products.length > 0 && products.every(product => field.supportedProductTypes.includes(product)));
  const applicableStages = [...new Set(products.map(stageForProduct))];

  function clearPreview() {
    setError("");
    onChange();
  }

  function toggleProduct(product: string, enabled: boolean) {
    setProducts(previous => enabled ? [...previous, product] : previous.filter(item => item !== product));
    clearPreview();
  }

  function updateCondition(id: number, change: Partial<DraftCondition>) {
    setConditions(previous => previous.map(condition => condition.id === id ? { ...condition, ...change } : condition));
    clearPreview();
  }

  function preview(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    try {
      if (!name.trim()) throw new Error("Enter a rule name.");
      if (!action) throw new Error("Choose the action to take when the rule matches.");
      if (!products.length || products.some(product => !profile.products.includes(product))) {
        throw new Error("Choose at least one product in the merchant’s catalogue.");
      }
      const conditionsForSave = conditions.map(condition => {
        const field = fieldOptions.find(candidate => candidate.fieldName === condition.field);
        if (!field || !field.allowedOperators.includes(condition.operator)) {
          throw new Error("Choose an available field and comparison for every condition.");
        }
        if (field.fieldName === "product.type" && !products.includes(condition.value)) {
          throw new Error("Choose a product included in the rule’s scope.");
        }
        return { field: condition.field, operator: condition.operator, value: conditionValue(field, condition.value) };
      });
      setError("");
      onPreview({
        scope: "MERCHANT", name: name.trim(), product_types: products, stages: applicableStages,
        conditions: { logic, rules: conditionsForSave }, action,
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Review the rule conditions.");
    }
  }

  if (profile.products.length === 0) return <p role="status" className="text-sm text-muted">Add a product to the merchant’s setup before creating a rule.</p>;

  return <form className="space-y-4" onSubmit={preview}>
    <fieldset disabled={disabled} className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-sm font-medium">Rule name
          <input className="control mt-1.5" value={name} maxLength={80} required onChange={event => {
            setName(event.currentTarget.value);
            clearPreview();
          }} />
        </label>
        <label className="text-sm font-medium">Action
          <select className="control mt-1.5" value={action} onChange={event => {
            setAction(event.currentTarget.value as PolicyAction | "");
            clearPreview();
          }}>
            {!action && <option value="">Choose an action</option>}
            {actions.map(value => <option key={value} value={value}>{policyActionLabels[value]}</option>)}
          </select>
        </label>
      </div>
      <fieldset>
        <legend className="text-sm font-medium">Products</legend>
        <div className="mt-1.5 flex flex-wrap gap-2">
          {profile.products.map(product => <label key={product} className="flex items-center gap-2 rounded-md border border-line px-2.5 py-1.5 text-sm">
            <input type="checkbox" checked={products.includes(product)} onChange={event => toggleProduct(product, event.currentTarget.checked)} />
            {productLabels[product] ?? product}
          </label>)}
        </div>
        <p className="mt-1.5 text-xs text-muted">Stages{applicableStages.length ? ` · ${applicableStages.map(stage => stages[stage]).join(", ")}` : " follow the selected products."}</p>
      </fieldset>
      <fieldset className="space-y-3">
        <legend className="mb-1 text-sm font-semibold">Conditions</legend>
        <label className="block max-w-xs text-sm font-medium">Match
          <select className="control mt-1.5" value={logic} onChange={event => {
            setLogic(event.currentTarget.value as "AND" | "OR");
            clearPreview();
          }}>
            <option value="AND">All conditions (AND)</option>
            <option value="OR">Any condition (OR)</option>
          </select>
        </label>
        {conditions.map((condition, index) => {
          const field = fieldOptions.find(option => option.fieldName === condition.field);
          return <fieldset key={condition.id} className="rounded-lg border border-line bg-paper p-3">
            <legend className="px-1 text-xs font-semibold">Condition {index + 1}</legend>
            <div className="grid gap-2.5 sm:grid-cols-3">
              <label className="min-w-0 text-sm font-medium">Field
                <select className="control mt-1.5" value={condition.field} onChange={event => {
                  const field = fieldOptions.find(option => option.fieldName === event.currentTarget.value);
                  updateCondition(condition.id, { field: field?.fieldName ?? "", operator: field?.allowedOperators[0] ?? "", value: "" });
                }}>
                  <option value="">Select field</option>
                  {condition.field && !field && <option value={condition.field} disabled>{condition.field} (unavailable)</option>}
                  {fieldOptions.map(option => <option key={option.fieldName} value={option.fieldName}>{policyFieldLabel(option)}</option>)}
                </select>
              </label>
              <label className="min-w-0 text-sm font-medium">Comparison
                <select className="control mt-1.5" disabled={!field} value={condition.operator}
                  onChange={event => updateCondition(condition.id, { operator: event.currentTarget.value })}>
                  {!field?.allowedOperators.includes(condition.operator) && <option value={condition.operator}>Select comparison</option>}
                  {field?.allowedOperators.map(operator => <option key={operator} value={operator}>{merchantOperatorLabels[operator] ?? operator}</option>)}
                </select>
              </label>
              <ConditionValue field={field} value={condition.value} products={products} semantic={condition.operator === "semantic_match"}
                onChange={value => updateCondition(condition.id, { value })} />
            </div>
            {conditions.length > 1 && <button type="button" className="mt-2 text-xs font-semibold text-red-800 hover:underline" onClick={() => {
              setConditions(previous => previous.filter(item => item.id !== condition.id));
              clearPreview();
            }}>Remove condition {index + 1}</button>}
          </fieldset>;
        })}
        {conditions.length < 5 && <button type="button" className={outlineButton} onClick={() => {
          const id = nextId.current++;
          setConditions(previous => [...previous, { id, field: "", operator: "", value: "" }]);
          clearPreview();
        }}>Add condition</button>}
      </fieldset>
      {error && <p role="alert" className="text-sm text-red-800">{error}</p>}
      <button type="submit" className={primaryButton}>Review rule</button>
    </fieldset>
  </form>;
}
