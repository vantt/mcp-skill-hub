import { useRoutes } from 'react-router';
import { AppShell } from './components/AppShell';
import { routes } from './routes';

export function App() {
  const element = useRoutes(routes);
  return <AppShell>{element}</AppShell>;
}
