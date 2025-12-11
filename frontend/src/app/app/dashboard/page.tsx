export default function DashboardPage() {
  return (
    <div className="flex flex-1 flex-col gap-4 p-4 pt-0">
      <h1 className="text-2xl font-bold ml-2">Dashboard</h1>
      <div className="grid auto-rows-min gap-4 md:grid-cols-3">
        <div className="bg-gray-200 aspect-video rounded-xl" />
        <div className="bg-gray-200 aspect-video rounded-xl" />
        <div className="bg-gray-200 aspect-video rounded-xl" />
      </div>
      <div className="flex w-full bg-gray-200 min-h-24 flex-1 rounded-xl" />
    </div>
  );
}
