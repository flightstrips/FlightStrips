---
title: GE + GW
description: Kastrup Ground East + West — tower ground scope, bays, and strip flows.
sidebar:
  order: 6
---

![Kastrup Ground East + West](../../../assets/ekch/ge-gw.jpg)

**GE + GW** is the tower ground scope for **EKCH_C_TWR** (118.580), using the **TWR aspect** layout. It covers runway and taxi logic while coordinating with apron and delivery.

Strips live in **bays** that are **ACTIVE** or **LOCKED**. Use **REQ** where the bay allows; fully locked strip types cannot be requested.

## Bay overview

| Bay | Strip type | Notes |
| --- | --- | --- |
| **Messages** | Messages | Coordination / free-text. |
| **Final** | `TWR-ARR` | Arrival finals — force assume gives GE+GW ownership. See [TE + TW](/ekch/te-tw/) for detail. |
| **RWY ARR** | `TWR-ARR` | Runway arrival segment. Can be REQ. |
| **TWY ARR** | `APN-ARR` | Apron arrival taxi. Can be REQ. |
| **Startup** | `APNPUSH` | Cleared departures; labelled **STARTUP-TWR**. If any apron position is online, limited to stands **G110–G137**, **W1**, and **AS**; otherwise all stands. |
| **Push back (TWR)** | `APNPUSH` | Release / direction from the pushback map moves the strip here; opens the TWR taxi map. |
| **TWY DEP (TWR)** | `TWR-DEP` | Synced with apron TWY DEP-LWR; may use different strip designs. Can be REQ from TWY DEP-LWR. SI to next sector applies. |
| **RWY DEP** | `TWR-DEP` | Departures on the runway segment. |
| **Airborne** | `TWR-DEP` | After departure. |
| **De-ice A** | `APNPUSH` | Same strip family; mostly manual moves between active bays. |
| **CLR DEL** | *(passive)* | Passive if CLR DEL, DEL+SEQ, or SEQ PLN is online. |
| **Stand** | `APN-ARR` | Gate / stand; active. |

## Departures

Startup → Push back (TWR) via pushback map → TWY DEP (TWR) / De-ice A as needed. TWY DEP (TWR) and apron TWY DEP-LWR stay in sync; use SI and handoff rules per spec. RWY DEP and Airborne behaviour (including auto-move and SI) aligns with [TE + TW](/ekch/te-tw/).

## Arrivals

TWY ARR strips match other TWR scopes; SI splits follow system logic. Final and RWY ARR (`TWR-ARR`) landing workflow and runway colour rules are documented in [TE + TW](/ekch/te-tw/).

## Uncleared strips

All uncleared strips sit in **CLR DEL** (no airline split in this scope), with the same behaviour as [CLR DEL](/ekch/clr-del/).

## Related

- [AA + AD](/ekch/aa-ad/) — combined apron scope
- [TE + TW](/ekch/te-tw/) — tower east + west (finals, runway, airborne detail)
