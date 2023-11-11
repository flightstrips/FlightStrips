export function Button(props: { title: string }) {
  return (
    <button className="flex bg-gray-700 border-gray-300 border-2 w-fit ml-2 mr-2 pl-2 pr-2 h-3/4 text-white justify-center items-center font-bold text-lg">
      {props.title}
    </button>
  )
}
