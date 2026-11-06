import {getApiUrl} from "@/lib/api-url";
import {fetchWithAccessTokenRetry} from "@/lib/authenticated-fetch";

export interface BoardCapabilities {
  cdm: boolean;
  stands: boolean;
  text_pilot: boolean;
  pdc: boolean;
  aman: boolean;
}

export interface OwnMembership {
  instance_id: string;
  membership_id: string;
  transport_generation: string;
  generation: string;
  session_id: number;
  airport: string;
  environment: string;
  role: "master" | "slave" | "observer";
  read_only: boolean;
  ready: boolean;
  master_epoch: string;
  view: string;
  definition_revision: string;
  actions: string[];
  capabilities: BoardCapabilities;
  callsign: string;
  position: string;
}

export interface SessionDiscovery {
  transports: Array<{
    instance_id: string;
    transport_generation: string;
    memberships: OwnMembership[];
  }>;
}

export interface AuthorizedSession {
  session_id: number;
  airport: string;
  environment: string;
  instances: OwnMembership[];
}

// Views are compiled frontend code. Unsupported views cannot enter an EKCH board.
export function authorizedSessions(discovery: SessionDiscovery): AuthorizedSession[] {
  const sessions = new Map<number, AuthorizedSession>();
  for (const transport of discovery.transports) {
    for (const member of transport.memberships) {
      if (member.view !== "ekch" || !member.definition_revision) continue;
      let session = sessions.get(member.session_id);
      if (!session) {
        session = {session_id: member.session_id, airport: member.airport, environment: member.environment, instances: []};
        sessions.set(member.session_id, session);
      }
      session.instances.push(member);
    }
  }
  return [...sessions.values()].sort((a, b) => a.airport.localeCompare(b.airport) || a.environment.localeCompare(b.environment) || a.session_id - b.session_id);
}

export function membershipKey(member: OwnMembership): string {
  return JSON.stringify([member.session_id, member.instance_id, member.transport_generation, member.membership_id, member.generation]);
}

export async function discoverSessions(
  getToken: (options?: {cacheMode?: "on" | "off" | "cache-only"}) => Promise<string | undefined>,
  signal?: AbortSignal,
): Promise<AuthorizedSession[]> {
  const response = await fetchWithAccessTokenRetry(getToken, getApiUrl("/api/plugin-memberships"), {signal, cache: "no-store"});
  if (!response.ok) throw new Error("Unable to discover authorized airport sessions");
  return authorizedSessions(await response.json() as SessionDiscovery);
}
