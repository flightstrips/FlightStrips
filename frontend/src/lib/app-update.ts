import { registerSW } from "virtual:pwa-register";

const UPDATE_CHECK_INTERVAL_MS = 120_000;
const FORCE_RELOAD_DELAY_MS = 3_000;
const VERSION_ENDPOINT = "/version.json";

type UpdateSource = "deployment-version" | "service-worker";

type UpdateState = {
  available: boolean;
  source: UpdateSource | null;
};

type UpdateListener = (state: UpdateState) => void;

const listeners = new Set<UpdateListener>();
const initialDeploymentVersion = window.__APP_CONFIG__?.deploymentVersion?.trim() ?? "";

let currentState: UpdateState = {
  available: false,
  source: null,
};
let monitoringStarted = false;
let forcedReloadScheduled = false;
let forcedReloadInProgress = false;
let serviceWorkerUpdateReady = false;
let updateServiceWorker: ((reloadPage?: boolean) => Promise<void>) | null = null;

function emitState() {
  for (const listener of listeners) {
    listener(currentState);
  }
}

function markUpdateAvailable(source: UpdateSource) {
  if (currentState.available && currentState.source === source) {
    return;
  }

  currentState = {
    available: true,
    source,
  };
  emitState();
  scheduleForcedReload();
}

function scheduleForcedReload() {
  if (forcedReloadScheduled || forcedReloadInProgress) {
    return;
  }

  forcedReloadScheduled = true;
  const delay = document.visibilityState === "visible" ? FORCE_RELOAD_DELAY_MS : 0;

  window.setTimeout(() => {
    void forceAppReload();
  }, delay);
}

async function forceAppReload() {
  if (forcedReloadInProgress) {
    return;
  }

  forcedReloadInProgress = true;
  await reloadForAppUpdate();
}

async function checkDeploymentVersion() {
  if (!initialDeploymentVersion) {
    return;
  }

  try {
    const response = await fetch(VERSION_ENDPOINT, { cache: "no-store" });
    if (!response.ok) {
      console.warn("Unable to check the deployed frontend version.");
      return;
    }

    const payload = await response.json() as { deploymentVersion?: unknown };
    const latestDeploymentVersion = typeof payload.deploymentVersion === "string"
      ? payload.deploymentVersion.trim()
      : "";

    if (latestDeploymentVersion && latestDeploymentVersion !== initialDeploymentVersion) {
      markUpdateAvailable("deployment-version");
    }
  } catch (error) {
    console.error("Failed to check for a newer frontend deployment:", error);
  }
}

function startDeploymentVersionMonitoring() {
  if (!initialDeploymentVersion) {
    return;
  }

  const checkWhenVisible = () => {
    if (document.visibilityState === "visible") {
      void checkDeploymentVersion();
    }
  };

  void checkDeploymentVersion();
  window.addEventListener("focus", checkWhenVisible);
  window.addEventListener("online", checkWhenVisible);
  document.addEventListener("visibilitychange", checkWhenVisible);
  window.setInterval(checkWhenVisible, UPDATE_CHECK_INTERVAL_MS);
}

function startServiceWorkerMonitoring() {
  updateServiceWorker = registerSW({
    immediate: true,
    onNeedRefresh() {
      serviceWorkerUpdateReady = true;
      markUpdateAvailable("service-worker");
    },
    onRegisteredSW(_swUrl, registration) {
      if (!registration) {
        return;
      }

      window.setInterval(() => {
        void registration.update();
      }, UPDATE_CHECK_INTERVAL_MS);
    },
    onRegisterError(error) {
      console.error("Failed to register the frontend service worker:", error);
    },
  });
}

export function startAppUpdateMonitoring() {
  if (monitoringStarted) {
    return;
  }

  monitoringStarted = true;
  startServiceWorkerMonitoring();
  startDeploymentVersionMonitoring();
}

export function subscribeToAppUpdates(listener: UpdateListener) {
  startAppUpdateMonitoring();
  listeners.add(listener);
  listener(currentState);

  return () => {
    listeners.delete(listener);
  };
}

export async function reloadForAppUpdate() {
  if (serviceWorkerUpdateReady && updateServiceWorker) {
    try {
      await updateServiceWorker(true);
      return;
    } catch (error) {
      console.error("Failed to activate the updated service worker:", error);
    }
  }

  window.location.reload();
}
