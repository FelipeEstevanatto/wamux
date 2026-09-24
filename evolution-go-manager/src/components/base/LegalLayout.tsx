import React from 'react';
import { Link } from 'react-router-dom';
import { ArrowLeft } from 'lucide-react';

import { COPYRIGHT_LINE, FORK_DISCLAIMER } from '@/constants/branding';

interface LegalLayoutProps {
  title: string;
  subtitle: string;
  updatedAt: string;
  children: React.ReactNode;
}

/**
 * Shared shell for the public legal pages (Termos de Serviço / Política de
 * Privacidade). They are intentionally reachable without authentication so the
 * links shown on the login screen lead somewhere real.
 */
export const LegalLayout: React.FC<LegalLayoutProps> = ({
  title,
  subtitle,
  updatedAt,
  children,
}) => (
  <div className="min-h-screen bg-gradient-to-t from-primary/20 via-background/95 to-background">
    <div className="container mx-auto max-w-3xl px-4 py-10 sm:py-14">
      <Link
        to="/manager/login"
        className="inline-flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Voltar para o login
      </Link>

      <header className="mt-6">
        <h1 className="text-3xl font-bold text-primary">{title}</h1>
        <p className="mt-2 text-muted-foreground">{subtitle}</p>
        <p className="mt-1 text-xs text-muted-foreground">
          Última atualização: {updatedAt}
        </p>
      </header>

      <div className="mt-8 space-y-7 rounded-lg border bg-background/80 p-6 shadow-lg backdrop-blur-sm">
        {children}
      </div>

      <footer className="mt-8 space-y-1 text-center text-xs text-muted-foreground">
        <p>{COPYRIGHT_LINE}</p>
        <p>{FORK_DISCLAIMER}</p>
      </footer>
    </div>
  </div>
);

interface LegalSectionProps {
  title: string;
  children: React.ReactNode;
}

export const LegalSection: React.FC<LegalSectionProps> = ({
  title,
  children,
}) => (
  <section className="space-y-2">
    <h2 className="text-lg font-semibold text-foreground">{title}</h2>
    <div className="space-y-2 text-sm leading-relaxed text-muted-foreground">
      {children}
    </div>
  </section>
);

export default LegalLayout;
