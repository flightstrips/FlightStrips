---
title: SO Landing Logic
description: Decision flow for assigning arriving aircraft to APRON or STAND outcomes.
---

This page describes the SO flowchart for inbound aircraft and how each path leads to the final handling state.

## High-level sequence

1. Aircraft starts at **TE+TW FINAL**.
2. Progresses to **TE+TW RWY ARR**.
3. Progresses to **TE+TW TWY ARR**.
4. Parking context and stand-type combinations then decide whether the strip remains unconcerned, moves to stand handling, or joins apron-arrival handling.

## Decision logic by branch

### 1) From `TE+TW TWY ARR` to initial combinations

The flow branches by parking location and stand context:

- **Parked on CARGO** leads to **GE**.
- **Parked on APRON (22R/04L/12 for LDG)** leads to **GW**.
- **Parked on APRON (22L for LDG, 12 feed)** leads to **AA**.
- **Parked on APRON (22L/04R/30 for LDG, TWY 8 feed)** also leads to **AA**.

These outcomes create two working groups:

- **GE + GW** (cargo/apron mixed non-AA path)
- **AA** (apron-arrival path)

### 2) GE + GW path

`GE` and `GW` merge into **GE+GW TWY ARR**, then split again by current parking:

- If **Parked on CARGO**:
  - Move to **GE+GW STAND**.
  - Also mark as **UNCONCERNED for APRON** (left branch in the chart).
- If **Parked on APRON**:
  - Re-classify to **AA** and then join the apron-arrival stream.

### 3) AA / apron-arrival path

All AA-origin and AA-converted traffic converges to:

- **APRON ARR / AA+AD TWY ARR**
- then to **APRON ARR / AA+AD STAND** (final state)

## Practical interpretation

- **Cargo-side GE/GW** traffic can end in a dedicated stand state and be treated as not relevant for apron concern.
- **Any APRON-routed traffic** (direct AA or GE/GW converted to AA) is funneled into the **APRON ARR / AA+AD** sequence until stand.
