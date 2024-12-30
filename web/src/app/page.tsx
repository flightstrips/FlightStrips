import Image from "next/image";

export default function Home() {
  return (
    <div className="grid grid-rows-[20px_1fr_20px] items-center justify-items-center min-h-screen p-8 pb-20 gap-16 sm:p-20 font-[family-name:var(--font-geist-sans)]">
      <main className="flex flex-col row-start-2 items-center justify-center">
        <h1 className="text-4xl">Flightstrips</h1>
        <p>Next-gen virtual strip management for VATSIM</p>
      </main>
    </div>
  );
}
