import { useEffect, useState } from "react";
import { useLocation, useNavigation } from "react-router";

export default function RouteIndicator() {
  const location = useLocation();
  const navigation = useNavigation();
  const [routeHistory, setRouteHistory] = useState<
    Array<{
      path: string;
      timestamp: string;
      type: "initial" | "navigation";
    }>
  >([]);
  const [isExpanded, setIsExpanded] = useState(false);
  const [pageLoadTime, setPageLoadTime] = useState<number | null>(null);

  useEffect(() => {
    const startTime = performance.now();

    // eslint-disable-next-line react-hooks/set-state-in-effect
    setRouteHistory((prev) => {
      const newEntry = {
        path: location.pathname,
        timestamp: new Date().toLocaleTimeString(),
        type:
          prev.length === 0 ? ("initial" as const) : ("navigation" as const),
      };

      const updated = [...prev, newEntry].slice(-10);
      return updated;
    });

    requestAnimationFrame(() => {
      const endTime = performance.now();
      setPageLoadTime(endTime - startTime);
    });
  }, [location.pathname]);

  const isNavigating = navigation.state !== "idle";
  const currentRoute = location.pathname === "/" ? "/" : location.pathname;

  return (
    <div className="fixed bottom-4 right-4 z-50">
      {isNavigating && (
        <div className="mb-2 bg-red-500 text-white px-4 py-2 rounded-lg shadow-lg animate-pulse">
          <div className="flex items-center gap-2">
            <div className="w-2 h-2 bg-white rounded-full animate-ping"></div>
            <span className="text-sm font-medium">Navigating...</span>
          </div>
        </div>
      )}

      <div className="bg-white/95 backdrop-blur-sm border border-gray-200 rounded-lg shadow-2xl overflow-hidden">
        <button
          onClick={() => setIsExpanded(!isExpanded)}
          className="w-full px-4 py-3 flex items-center justify-between hover:bg-gray-50 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="relative">
              <div className="w-3 h-3 bg-green-500 rounded-full animate-pulse"></div>
              <div className="absolute inset-0 w-3 h-3 bg-green-500 rounded-full animate-ping"></div>
            </div>
            <div className="text-left">
              <div className="text-xs text-gray-500">Client Routing</div>
              <div className="text-sm font-mono text-gray-900">
                {currentRoute}
              </div>
            </div>
          </div>
          <svg
            className={`w-5 h-5 text-gray-500 transition-transform ${
              isExpanded ? "rotate-180" : ""
            }`}
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={2}
              d="M19 9l-7 7-7-7"
            />
          </svg>
        </button>

        {isExpanded && (
          <div className="border-t border-gray-200">
            <div className="px-4 py-3 bg-gray-50">
              <div className="text-xs text-gray-600 mb-2">
                Performance Metrics
              </div>
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="bg-green-50 border border-green-200 rounded px-2 py-1">
                  <div className="text-green-600 font-medium">
                    {pageLoadTime !== null
                      ? `${pageLoadTime.toFixed(2)}ms`
                      : "Measuring..."}
                  </div>
                  <div className="text-gray-600">Load Time</div>
                </div>
                <div className="bg-blue-50 border border-blue-200 rounded px-2 py-1">
                  <div className="text-blue-600 font-medium">
                    {routeHistory.length}
                  </div>
                  <div className="text-gray-600">Navigations</div>
                </div>
              </div>
            </div>

            <div className="px-4 py-3">
              <div className="text-xs text-gray-600 mb-2">Route History</div>
              <div className="space-y-1 max-h-48 overflow-y-auto">
                {routeHistory
                  .slice()
                  .reverse()
                  .map((entry, index) => (
                    <div
                      key={`${entry.timestamp}-${index}`}
                      className={`text-xs p-2 rounded ${
                        index === 0
                          ? "bg-green-50 border border-green-200"
                          : "bg-gray-50"
                      }`}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="font-mono text-gray-900 truncate flex-1">
                          {entry.path === "/" ? "/" : entry.path}
                        </span>
                        <span className="text-gray-500 whitespace-nowrap">
                          {entry.timestamp}
                        </span>
                      </div>
                      {entry.type === "initial" && (
                        <div className="text-gray-500 mt-1">Initial Load</div>
                      )}
                    </div>
                  ))}
              </div>
            </div>

            <div className="px-4 py-3 bg-gray-50 border-t border-gray-200">
              <div className="text-xs text-gray-600 space-y-1">
                <div className="flex items-center gap-2">
                  <div className="w-2 h-2 bg-green-500 rounded-full"></div>
                  <span>Client Routing: No page refresh</span>
                </div>
                <div className="flex items-center gap-2">
                  <div className="w-2 h-2 bg-red-500 rounded-full"></div>
                  <span>TBA: State Persistence: Component state preserved</span>
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
