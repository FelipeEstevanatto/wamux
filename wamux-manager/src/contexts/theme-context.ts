import { createContext } from 'react';

export type Theme = 'light' | 'dark';

export interface DarkModeContextType {
  theme: Theme;
  toggleTheme: () => void;
}

/** Kept outside the provider module so Fast Refresh only sees components. */
export const DarkModeContext = createContext<DarkModeContextType | undefined>(undefined);
