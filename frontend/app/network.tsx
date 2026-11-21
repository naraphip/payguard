"use client";

import { useEffect, useRef } from "react";
import type { ElementDefinition } from "cytoscape";
import type { Decision, Entity, EntityKind } from "./types";

const palette: Record<EntityKind, { label: string; color: string }> = {
  account: { label: "Account", color: "#3d5bda" },
  device: { label: "Device", color: "#8350c5" },
  ip: { label: "IP address", color: "#177d97" },
  token: { label: "Payment token", color: "#a15d21" },
};

function nodeID(entity: Entity) {
  return `${entity.kind}:${entity.key}`;
}

export function Network({ selected, decisions }: { selected: Decision; decisions: Decision[] }) {
  const container = useRef<HTMLDivElement>(null);
  const [account, ...signals] = selected.event.entities;
  const links = signals.map(signal => {
    const peers = new Map<string, Entity>();
    for (const { event } of decisions) {
      const [other] = event.entities;
      if (other.key !== account.key && event.entities.some(entity => nodeID(entity) === nodeID(signal))) {
        peers.set(other.key, other);
      }
    }
    return { signal, peers: [...peers.values()] };
  });

  useEffect(() => {
    let disposed = false;
    let destroy: (() => void) | undefined;
    const [focus, ...entities] = selected.event.entities;
    const elements = new Map<string, ElementDefinition>();
    const addNode = (entity: Entity, label: string) => {
      const id = nodeID(entity);
      elements.set(id, { data: { id, label, color: palette[entity.kind].color } });
    };
    const addEdge = (a: Entity, b: Entity) => {
      const source = nodeID(a);
      const target = nodeID(b);
      const id = source + "/" + target;
      elements.set(id, { data: { id, source, target } });
    };

    addNode(focus, "Selected account");
    for (const signal of entities) {
      addNode(signal, palette[signal.kind].label);
      addEdge(focus, signal);
      const peers = new Set<string>();
      for (const { event } of decisions) {
        const [other] = event.entities;
        if (other.key === focus.key || peers.has(other.key) || !event.entities.some(entity => nodeID(entity) === nodeID(signal))) {
          continue;
        }
        peers.add(other.key);
        addNode(other, "Acct " + other.key.slice(0, 5));
        addEdge(other, signal);
        if (peers.size === 5) break;
      }
    }

    import("cytoscape").then(({ default: cytoscape }) => {
      if (disposed || !container.current) return;
      const graph = cytoscape({
        container: container.current,
        elements: [...elements.values()],
        layout: { name: "breadthfirst", roots: [nodeID(focus)], directed: false, padding: 28 },
        minZoom: 0.3,
        maxZoom: 2,
        style: [
          { selector: "node", style: {
            "background-color": "data(color)", width: 17, height: 17, label: "data(label)",
            "text-valign": "bottom", "text-margin-y": 5, "font-size": 11, color: "#37445d",
          } },
          { selector: "edge", style: { width: 1, "line-color": "#b4bfd2", "curve-style": "straight" } },
        ],
      });
      destroy = () => graph.destroy();
    });

    return () => { disposed = true; destroy?.(); };
  }, [selected, decisions]);

  return (
    <div>
      <div aria-hidden="true" ref={container} className="graph h-60 rounded-lg border border-line" />
      <p className="mt-3 text-xs text-muted">Edges come from simulation events. The graph shows up to five other accounts per shared entity.</p>
      <ul className="mt-3 grid gap-2 sm:grid-cols-3">
        {links.map(({ signal, peers }) => (
          <li key={nodeID(signal)} className="min-w-0 rounded-lg border border-line p-3 text-xs">
            <span className="font-semibold" style={{ color: palette[signal.kind].color }}>{palette[signal.kind].label}</span>
            <code className="mt-1 block truncate text-muted" title={signal.key}>{signal.key}</code>
            <span className="mt-1 block tabular-nums">{peers.length} other accounts</span>
          </li>
        ))}
      </ul>
    </div>
  );
}
