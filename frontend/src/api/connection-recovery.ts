export type ConnectionPhase = "disconnected" | "connecting" | "authenticating" | "synchronizing" | "waiting_for_plugin" | "ready" | "reconnecting";

/** A watchdog belongs to exactly one physical socket, never to a board mutation. */
export class ConnectionRecovery {
  private deadline: ReturnType<typeof setTimeout> | null = null;
  private heartbeat: ReturnType<typeof setInterval> | null = null;
  private supported = false;
  private pendingSince = 0;
  private sequence = 0;
  private pending: string | null = null;
  private connectionID: string = crypto.randomUUID();
  private ray = "";
  private phase: ConnectionPhase = "connecting";
  private stopped = false;

  constructor(private readonly callbacks: {
    send: (message: Record<string, unknown>) => void;
    fail: (reason: string) => void;
    phase: (phase: ConnectionPhase) => void;
    previousFailure: string;
    attempt: number;
  }) {
    this.transition("connecting");
    this.armDeadline("connect_timeout", 15_000);
  }
  private transition(phase: ConnectionPhase): void {
    this.phase = phase;
    console.info("WebSocket recovery", {phase, connection_id: this.connectionID, cf_ray: this.ray, attempt: this.callbacks.attempt});
    this.callbacks.phase(phase);
  }
  private armDeadline(reason: string, ms: number): void {
    if (this.deadline) clearTimeout(this.deadline);
    this.deadline = setTimeout(() => this.fail(reason), ms);
  }
  private fail(reason: string): void {
    if (this.stopped) return;
    console.warn("WebSocket recovery failed", {reason, phase: this.phase, connection_id: this.connectionID, cf_ray: this.ray});
    this.stop();
    this.callbacks.fail(reason);
  }
  opened(): void {this.transition("authenticating"); this.armDeadline("snapshot_timeout", 30_000);}
  ready(): void {
    if (this.deadline) clearTimeout(this.deadline);
    this.deadline = null;
    this.transition("ready");
  }
  waitingForPlugin(): void {
    if (this.deadline) clearTimeout(this.deadline);
    this.deadline = null;
    this.transition("waiting_for_plugin");
  }
  private sendHealth(snapshot = false): void {
    this.pending = String(++this.sequence);
    this.pendingSince = Date.now();
    try {this.callbacks.send({type: "connection_health", sequence: this.pending, stage: this.phase,
      previous_failure: this.callbacks.previousFailure, request_snapshot: snapshot});}
    catch {this.fail("socket_closed");}
  }
  receiveHealth(data: Record<string, unknown>): void {
    if (this.stopped || data.protocol !== 1 || typeof data.connection_id !== "string") return;
    if (!this.supported) {
      this.supported = true;
      this.connectionID = data.connection_id;
      this.ray = typeof data.cf_ray === "string" ? data.cf_ray : "";
      this.heartbeat = setInterval(() => {
        // A suspended tab has not necessarily sent a probe. Start the timeout
        // at transmission, rather than treating a delayed timer as packet loss.
        if (this.pending && Date.now() - this.pendingSince >= 30_000) {this.fail("heartbeat_timeout"); return;}
        if (!this.pending) this.sendHealth();
      }, 10_000);
    } else if (data.sequence === this.pending) {this.pending = null;}
    else return;
    if (data.waiting_for_plugin === true && this.phase !== "ready") this.waitingForPlugin();
    else if (this.phase === "waiting_for_plugin") {this.transition("synchronizing"); this.armDeadline("snapshot_timeout", 30_000);}
  }
  requestSnapshot(): boolean {
    if (this.stopped || !this.supported) return false;
    this.transition("synchronizing");
    this.armDeadline("snapshot_timeout", 30_000);
    this.sendHealth(true);
    return true;
  }
  stop(): void {
    this.stopped = true;
    if (this.deadline) clearTimeout(this.deadline);
    if (this.heartbeat) clearInterval(this.heartbeat);
    this.deadline = this.heartbeat = null;
  }
}
