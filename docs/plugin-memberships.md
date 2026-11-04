# Plugin membership protocol

MA-03 extends the existing typed binary envelope on the existing endpoint. The
backend and plugin must be deployed together during the agreed offline window.

`TokenEvent.instance_id` identifies one plugin process across reconnects. It is
not an authorization credential: the token supplies the authenticated CID.
The backend assigns a transport generation and a new membership ID/generation
for every admission. `Envelope.context` carries those values, the database
session, airport, complete environment identity, and current master epoch.
Queued messages capture this context at enqueue time. Receivers validate it
again when applying work. Neither a CID nor an airport alone identifies a source.

`ControlEvent` reports physical-instance identity and radio evidence, even with
zero airport memberships. It does not keep operational controllers or sessions
alive. `SessionInfoEvent` remains the typed role announcement, now scoped by the
envelope. `MembershipRemovedEvent` removes only its addressed incarnation.
Master epochs fence observations and full snapshots, while authorized controller
commands/results do not require mastership. There is no generic acknowledgement
or parallel protocol endpoint.

This first stack layer supplies the shared wire contracts and generated Go/C++
consumers. Transport admission, plugin state, application fences, and authorized
discovery are implemented by the following MA-03 layers.
