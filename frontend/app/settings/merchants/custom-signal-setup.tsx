"use client";

export type CustomSignalDefinition = { key: string; label: string; type: "text" | "number" | "boolean" | "country" };
const kinds: Record<CustomSignalDefinition["type"], string> = {
  text: "Text", number: "Whole number", boolean: "True or false", country: "Country code",
};

export function CustomSignalSetup({ fields, onChange }: {
  fields: CustomSignalDefinition[];
  onChange: (fields: CustomSignalDefinition[]) => void;
}) {
  function patch(index: number, change: Partial<CustomSignalDefinition>) {
    onChange(fields.map((field, i) => i === index ? { ...field, ...change } : field));
  }
  return <fieldset className="space-y-3">
    <legend className="text-sm font-semibold">Custom fraud signals</legend>
    <p className="text-xs text-muted">Register the non-identifying fields your integration supplies. These fields will be available in merchant rules, Mock Checkout and Simulation Lab.</p>
    {fields.map((field, index) => <div key={index} className="grid gap-2 rounded-lg border border-line p-3 sm:grid-cols-3">
      <label className="text-sm font-medium">Field key<input className="control mt-1" required pattern="custom_[a-z][a-z0-9_]{0,39}" maxLength={47}
        placeholder="custom_ticket_title" value={field.key} onChange={e => patch(index,{ key:e.currentTarget.value })} /></label>
      <label className="text-sm font-medium">Label<input className="control mt-1" required maxLength={60} placeholder="Ticket title"
        value={field.label} onChange={e => patch(index,{ label:e.currentTarget.value })} /></label>
      <label className="text-sm font-medium">Value type<select className="control mt-1" value={field.type}
        onChange={e => patch(index,{ type:e.currentTarget.value as CustomSignalDefinition["type"] })}>
        {Object.entries(kinds).map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></label>
      <button type="button" className="justify-self-start text-xs font-semibold text-red-700" onClick={() => onChange(fields.filter((_, i) => i !== index))}>Remove field</button>
    </div>)}
    <button type="button" disabled={fields.length >= 16} className="text-sm font-semibold text-accent disabled:text-muted"
      onClick={() => onChange([...fields,{ key: "custom_", label:"", type:"text" }])}>Add custom field</button>
  </fieldset>;
}
