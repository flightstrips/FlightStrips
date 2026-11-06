import {createContext} from "react";
import type {AuthorizedSession, OwnMembership} from "@/api/sessions";

export const AirportSessionContext = createContext<{
  selected: OwnMembership;
  sessions: AuthorizedSession[];
  openChooser: () => void;
} | null>(null);
