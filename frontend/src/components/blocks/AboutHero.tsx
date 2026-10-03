const stats = [
  { value: "NITOS", label: "Inspired by" },
  { value: "Any device", label: "Runs on" },
  { value: "DCL + more", label: "Features" },
  { value: "Open source", label: "Community" },
];

export function AboutStats() {
  return (
    <dl className="mt-14 grid grid-cols-2 border-l border-t border-[var(--fsl-line)] lg:grid-cols-4">
      {stats.map((stat) => (
        <div
          key={stat.label}
          className="fsl-hoverbar flex flex-col-reverse border-b border-r border-[var(--fsl-line)] bg-[var(--fsl-surface)] p-6"
        >
          <dt className="fsl-eyebrow mt-3">{stat.label}</dt>
          <dd className="fsl-display text-[22px] text-[var(--fsl-brand-ink)] sm:text-[26px]">{stat.value}</dd>
        </div>
      ))}
    </dl>
  );
}
