import type { RouteObject } from 'react-router';
import { NotFoundPage } from './components/NotFoundPage';
import { HomeScreen } from './screens/home/HomeScreen';
import { SkillsScreen } from './screens/skills/SkillsScreen';
import { AddSkillScreen } from './screens/skill-add/AddSkillScreen';
import { CreateSkillScreen } from './screens/skill-create/CreateSkillScreen';
import { SkillDetailScreen } from './screens/skill-detail/SkillDetailScreen';
import { SourcesScreen } from './screens/sources/SourcesScreen';
import { DistillHandoffScreen } from './screens/distill/DistillHandoffScreen';
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
    path: '/sources/distill',
    Component: DistillHandoffScreen,
  },
  {
    path: '*',
    Component: NotFoundPage,
  },
];
