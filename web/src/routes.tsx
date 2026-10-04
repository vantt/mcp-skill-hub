import type { RouteObject } from 'react-router';
import { LaterPhasePage } from './components/LaterPhasePage';
import { NotFoundPage } from './components/NotFoundPage';
import { HomeScreen } from './screens/home/HomeScreen';
import { SkillsScreen } from './screens/skills/SkillsScreen';

export const routes: RouteObject[] = [
  {
    path: '/',
    Component: HomeScreen,
  },
  {
    path: '/skills',
    Component: SkillsScreen,
  },
  {
    path: '/skills/add',
    Component: LaterPhasePage,
  },
  {
    path: '/skills/create',
    Component: LaterPhasePage,
  },
  {
    path: '/skills/:id',
    Component: LaterPhasePage,
  },
  {
    path: '/sources',
    Component: LaterPhasePage,
  },
  {
    path: '/sources/watch',
    Component: LaterPhasePage,
  },
  {
    path: '/sources/distill',
    Component: LaterPhasePage,
  },
  {
    path: '/sources/runs/:id',
    Component: LaterPhasePage,
  },
  {
    path: '/inbox',
    Component: LaterPhasePage,
  },
  {
    path: '/inbox/:id',
    Component: LaterPhasePage,
  },
  {
    path: '/inbox/:id/apply',
    Component: LaterPhasePage,
  },
  {
    path: '*',
    Component: NotFoundPage,
  },
];
