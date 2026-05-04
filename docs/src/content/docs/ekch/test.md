---
title: SO Landing Logic
description: Decision flow for assigning arriving aircraft to apron or stand outcomes.
---

## High-level sequence

1. Aircraft starts at **TE+TW Final**.
2. Progresses to **TE+TW RWY ARR**.
3. Progresses to **TE+TW TWY ARR**.
4. Parking context and stand type then decide whether the strip stays unconcerned, moves to stand handling, or joins apron-arrival handling.

## Decision logic

### From TWY ARR — initial branch

| Parking location | Outcome |
| --- | --- |
| Cargo | → **GE** |
| Apron (22R / 04L / 12 for LDG) | → **GW** |
| Apron (22L for LDG, 12 feed) | → **AA** |
| Apron (22L / 04R / 30 for LDG, TWY 8 feed) | → **AA** |

This produces two working groups: **GE + GW** and **AA**.

### GE + GW path

GE and GW merge into **GE+GW TWY ARR**, then split by parking:

- **Parked on Cargo** → move to **GE+GW STAND**; mark as unconcerned for apron.
- **Parked on Apron** → reclassify to **AA** and join the apron-arrival stream.

### AA / apron-arrival path

All AA-origin and AA-converted traffic flows to **Apron ARR / AA+AD TWY ARR**, then to **Apron ARR / AA+AD STAND** (final state).

## Summary

- Cargo-side GE/GW traffic ends in a stand state and is treated as not relevant for apron concern.
- Any apron-routed traffic (direct AA or GE/GW converted to AA) is funnelled through the **Apron ARR / AA+AD** sequence to stand.
