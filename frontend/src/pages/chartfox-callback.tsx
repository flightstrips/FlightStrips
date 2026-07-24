import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router';
import { completeChartFoxAuthorization } from '@/lib/chartfox-auth';

export default function ChartFoxCallbackPage() {
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    void completeChartFoxAuthorization(window.location.search)
      .then((returnTo) => navigate(returnTo, { replace: true }))
      .catch((reason: unknown) => setError(reason instanceof Error ? reason.message : 'ChartFox authorization failed.'));
  }, [navigate]);

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-4 bg-[#1d293d] p-6 text-center text-cyan-100">
      <h1 className="text-xl font-bold">CONNECTING TO CHARTFOX</h1>
      {error ? (
        <>
          <p role="alert" className="max-w-lg text-sm text-red-200">{error}</p>
          <button type="button" onClick={() => navigate('/efb', { replace: true })} className="border border-cyan-200 px-4 py-2 font-bold">BACK TO EFB</button>
        </>
      ) : <p className="text-sm">Please wait while FlightStrips securely connects your ChartFox account.</p>}
    </main>
  );
}
