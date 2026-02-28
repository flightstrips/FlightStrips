/**
 * Shared strip sub-components used across multiple flex-proportion strip variants.
 */

const CELL_BORDER = "border-r border-[#85b4af]";
const F_SI = 8;

/** SI / ownership indicator — 8% flex. Purple = unassumed, white = assumed, orange = transferred away. */
export function SIBox({
  owner,
  nextControllers,
  previousControllers,
  myIdentifier,
}: {
  owner?: string;
  nextControllers?: string[];
  previousControllers?: string[];
  myIdentifier?: string;
}) {
  const isAssumed = !!myIdentifier && owner === myIdentifier;
  const isTransferredAway =
    !!myIdentifier &&
    !!previousControllers?.includes(myIdentifier) &&
    !nextControllers?.includes(myIdentifier);

  let bgColor = "#E082E7";
  if (isAssumed) bgColor = "#F0F0F0";
  else if (isTransferredAway) bgColor = "#DD6A12";

  const nextLabel =
    isAssumed && nextControllers?.[0] ? nextControllers[0].slice(0, 2) : "";

  return (
    <div
      className={`flex items-center justify-center text-sm font-bold ${CELL_BORDER}`}
      style={{ flex: `${F_SI} 0 0%`, height: "100%", backgroundColor: bgColor, minWidth: 0 }}
    >
      {nextLabel}
    </div>
  );
}
