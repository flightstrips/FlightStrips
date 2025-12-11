import { useAuth0 } from "@auth0/auth0-react";

export default function ProfilePage() {
  const { user, isAuthenticated, isLoading } = useAuth0();

  if (isLoading) {
    return (
      <div className="flex flex-1 flex-col gap-4 p-4 pt-0">Loading ...</div>
    );
  }

  return (
    isAuthenticated &&
    user && (
      <div className="flex flex-1 flex-col gap-4 p-4 pt-0">
        <h1 className="text-2xl font-bold ml-2">Profile</h1>
        <div className="flex">
          <img src={user.picture} alt={user.name} className="rounded-md p-2" />
          <div className="flex flex-col justify-center items-start ml-2">
            <h2 className="text-primary text-2xl font-semibold">{user.name}</h2>
          </div>
        </div>
      </div>
    )
  );
}
