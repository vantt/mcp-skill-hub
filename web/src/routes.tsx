import type { RouteObject } from 'react-router';
import { LaterPhasePage } from './components/LaterPhasePage';
import { NotFoundPage } from './components/NotFoundPage';
import { HomeScreen } from './screens/home/HomeScreen';
import { SkillsScreen } from './screens/skills/SkillsScreen';
import { AddSkillScreen } from './screens/skill-add/AddSkillScreen';
import { CreateSkillScreen } from './screens/skill-create/CreateSkillScreen';
import { SkillDetailScreen } from './screens/skill-detail/SkillDetailScreen';
import { SourcesScreen } from './screens/sources/SourcesScreen';
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
    Component: AddSkillScreen,
  },
  {
    path: '/skills/create',
    Component: CreateSkillScreen,
  },
  {
    path: '/skills/:id',
    Component: SkillDetailScreen,
  },
  {
    path: '/sources',
    Component: SourcesScreen,
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
