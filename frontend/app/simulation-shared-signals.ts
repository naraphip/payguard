import type { Decision, EntityKind } from "./types";

type SimulationEvent = Decision["event"];
type SignalKind = Exclude<EntityKind, "account">;

const signalKinds: SignalKind[] = ["device", "ip", "token"];

function compareEvents(a: SimulationEvent, b: SimulationEvent) {
  // The engine breaks ties in the numeric event time by event ID.
  if (a.at !== b.at) return a.at - b.at;
  if (a.id === b.id) return 0;
  return a.id < b.id ? -1 : 1;
}

export function findSharedSignals(selected: SimulationEvent, decisions: Decision[]) {
  const account = selected.entities?.find(entity => entity.kind === "account")?.key;
  if (!account) return [];

  const earlierEvents = decisions
    .map(({ event }) => event)
    .filter(event => event.id !== selected.id && compareEvents(event, selected) < 0)
    .sort((a, b) => compareEvents(b, a));

  return signalKinds.flatMap(kind => {
    const keys = new Set(selected.entities.filter(entity => entity.kind === kind).map(entity => entity.key));
    if (keys.size === 0) return [];

    const seenAccounts = new Set([account]);
    const eventIds: string[] = [];
    for (const event of earlierEvents) {
      const other = event.entities?.find(entity => entity.kind === "account")?.key;
      if (!other || seenAccounts.has(other)) continue;
      if (!event.entities.some(entity => entity.kind === kind && keys.has(entity.key))) continue;

      seenAccounts.add(other);
      eventIds.push(event.id);
    }

    return eventIds.length ? [{ kind, eventIds }] : [];
  });
}
