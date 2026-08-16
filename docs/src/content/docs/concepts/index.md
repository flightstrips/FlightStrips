---
title: How Flight Strips works
description: The shared strip board, what controllers do, and what Flight Strips handles automatically.
sidebar:
  order: 1
---

Flight Strips is a shared electronic strip board. Every connected controller sees the same strip data, ownership and pending coordination. A controller records an operational decision on the strip; Flight Strips distributes that change and keeps the strip's workflow state synchronized.

The important distinction is:

- **You make operational decisions.** You clear, transfer, assume or update a strip when the real traffic situation requires it.
- **Flight Strips maintains workflow state.** It synchronizes changes, calculates the expected controller route, displays who is responsible next and performs the automatic transitions described in these docs.

Do not repeat an action merely because another controller's screen has not changed instantly. A transfer, clearance or other change is sent to the server and then broadcast to every client. Wait for the shared strip state to update before trying again.

## Learn the strip first

Before using the board, learn the field names and the presentations used during each stage of a flight. [Strip anatomy and types](/concepts/strip-anatomy/) lists every implemented strip presentation from left to right.

## Then learn ownership

Ownership answers three questions:

1. Who is responsible for the strip now?
2. Who is expected to receive it next?
3. Is a handoff waiting for another controller to act?

Read [Ownership and handoffs](/concepts/ownership/) before using transfers or tag requests.

## PDC is automatic until it needs judgement

A valid pre-departure clearance request without free-text remarks is issued automatically. A request with validation faults or remarks is held for a controller because Flight Strips cannot safely make that decision.

Read [Pre-departure clearance](/concepts/pre-departure-clearance/) to understand when to wait and when to intervene.
