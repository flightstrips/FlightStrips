export default function AboutPage() {
  return (
    <main className="bg-gray-100">
      <section className="relative">
        <div className="absolute -bottom-20 left-1/2 h-20 w-20 bg-gray-100 z-10 transform -translate-x-1/2">
          <div className="absolute bottom-0 left-1/2 mb-8">
            <div className="h-20 w-px bg-white" />
            <div className="h-12 w-px bg-gray-800" />
          </div>
        </div>
        <div className="relative bg-gray-900 text-white/90 overflow-hidden">
          <div className="container px-4 mx-auto">
            <div className="pt-20 pb-24 text-center">
              <h1 className="text-6xl py-4 font-semibold">About Us</h1>
              <p>Home / About Us</p>
            </div>
          </div>
        </div>
      </section>

      <section className="relative py-12 md:py-40">
        <div className="container px-4 mx-auto">
          <div className="flex gap-2 w-full justify-center">
            <div className="w-8 h-32 bg-linear-to-b from-primary to-transparent" />
            <h2 className="text-3xl font-semibold max-w-[24ch]">
              Our vision for a next generation strip management system
            </h2>
          </div>
        </div>
      </section>
    </main>
  );
}
