'use client';

import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import {
  Language,
  TranslationKey,
  getTranslation,
  getBrowserLanguage,
  getStoredLanguage,
  setStoredLanguage,
} from '@/lib/i18n';

interface LanguageContextType {
  language: Language;
  setLanguage: (lang: Language) => void;
  t: (key: TranslationKey) => string;
}

const LanguageContext = createContext<LanguageContextType | undefined>(undefined);

interface LanguageProviderProps {
  children: ReactNode;
}

export function LanguageProvider({ children }: LanguageProviderProps) {
  const [language, setLanguageState] = useState<Language>('en');
  const [isInitialized, setIsInitialized] = useState(false);

  useEffect(() => {
    // Initialize language from storage or browser
    const stored = getStoredLanguage();
    if (stored) {
      setLanguageState(stored);
    } else {
      const browserLang = getBrowserLanguage();
      setLanguageState(browserLang);
    }
    setIsInitialized(true);
  }, []);

  const setLanguage = (lang: Language) => {
    setLanguageState(lang);
    setStoredLanguage(lang);
  };

  const t = (key: TranslationKey): string => {
    return getTranslation(language, key);
  };

  // Prevent hydration mismatch by using default during SSR
  if (!isInitialized) {
    return (
      <LanguageContext.Provider value={{ language: 'en', setLanguage, t: (key) => getTranslation('en', key) }}>
        {children}
      </LanguageContext.Provider>
    );
  }

  return (
    <LanguageContext.Provider value={{ language, setLanguage, t }}>
      {children}
    </LanguageContext.Provider>
  );
}

export function useLanguage() {
  const context = useContext(LanguageContext);
  if (context === undefined) {
    throw new Error('useLanguage must be used within a LanguageProvider');
  }
  return context;
}
