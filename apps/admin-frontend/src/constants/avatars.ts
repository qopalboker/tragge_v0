// @deprecated - Avatars are now loaded from API. This file is kept for backward compatibility.
// Use profileApi.listAvatars() from @/modules/user/api instead.
// Predefined avatar options
export interface Avatar {
  id: string;
  name: string;
  nameKey: string;
  path: string;
  category: 'animal' | 'character' | 'special';
  bgColor: string;
}

export type AvatarOption = Avatar;
export const PREDEFINED_AVATARS: Avatar[] = [
  { id: 'shark', name: 'Business Shark', nameKey: 'avatar.shark', path: '/avatars/shark.png', category: 'animal', bgColor: '#1e3a5f' },
  { id: 'monkey', name: 'Diamond Monkey', nameKey: 'avatar.monkey', path: '/avatars/monkey.png', category: 'animal', bgColor: '#4a3728' },
  { id: 'snake', name: 'Viper', nameKey: 'avatar.snake', path: '/avatars/snake.png', category: 'animal', bgColor: '#2d4a2d' },
  { id: 'lion', name: 'King Lion', nameKey: 'avatar.lion', path: '/avatars/lion.png', category: 'animal', bgColor: '#8b6914' },
  { id: 'dragon', name: 'Dragon', nameKey: 'avatar.dragon', path: '/avatars/dragon.png', category: 'animal', bgColor: '#4a1a1a' },
  { id: 'panda', name: 'Tech Panda', nameKey: 'avatar.panda', path: '/avatars/panda.png', category: 'animal', bgColor: '#2d2d2d' },
  { id: 'eagle', name: 'Cyber Eagle', nameKey: 'avatar.eagle', path: '/avatars/eagle.png', category: 'animal', bgColor: '#1a1a3a' },
  { id: 'phoenix', name: 'Phoenix', nameKey: 'avatar.phoenix', path: '/avatars/phoenix.png', category: 'special', bgColor: '#4a2a0a' },
  { id: 'wolf', name: 'Shadow Wolf', nameKey: 'avatar.wolf', path: '/avatars/wolf.png', category: 'animal', bgColor: '#2a2a3a' },
  { id: 'samurai', name: 'Samurai', nameKey: 'avatar.samurai', path: '/avatars/samurai.png', category: 'character', bgColor: '#3a1a1a' },
  { id: 'bull', name: 'Bull', nameKey: 'avatar.bull', path: '/avatars/bull.png', category: 'animal', bgColor: '#3a2a1a' },
  { id: 'cat', name: 'VR Cat', nameKey: 'avatar.cat', path: '/avatars/cat.png', category: 'animal', bgColor: '#2a3a4a' },
  { id: 'bear', name: 'Bear', nameKey: 'avatar.bear', path: '/avatars/bear.png', category: 'animal', bgColor: '#4a3a2a' },
  { id: 'fox', name: 'Smart Fox', nameKey: 'avatar.fox', path: '/avatars/fox.png', category: 'animal', bgColor: '#5a3a1a' },
  { id: 'owl', name: 'Night Owl', nameKey: 'avatar.owl', path: '/avatars/owl.png', category: 'animal', bgColor: '#1a2a3a' },
  { id: 'robot', name: 'Trading Bot', nameKey: 'avatar.robot', path: '/avatars/robot.png', category: 'special', bgColor: '#2a3a4a' },
];

export const AVATARS = PREDEFINED_AVATARS;

export const getAvatarById = (id: string): Avatar | undefined => {
  return AVATARS.find((avatar) => avatar.id === id);
};

export const getAvatarsByCategory = (category: Avatar['category']): Avatar[] => {
  return AVATARS.filter((avatar) => avatar.category === category);
};
