import { Navigate } from 'react-router-dom';
import { userHome } from '@/lib/user-home';
import { useAuthStore } from '@/store/auth-store';

export function HomeRedirect() {
  const user = useAuthStore(state => state.user);
  return <Navigate to={userHome(user)} replace />;
}
