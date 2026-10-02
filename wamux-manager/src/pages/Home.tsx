import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui';
import { ArrowRight, Zap, Shield, Globe, Server } from 'lucide-react';
import apiClient from '@/services/api/client';
import { COPYRIGHT_LINE, PRODUCT_NAME } from '@/constants/branding';
import { useI18n } from '@/i18n/I18nContext';

interface RootInfo {
  status?: number;
  message?: string;
  version?: string;
  clientName?: string;
  manager?: string;
  documentation?: string;
  whatsappWebVersion?: string;
  error?: string;
}

export const Home: React.FC = () => {
  const navigate = useNavigate();
  const { t } = useI18n();
  const [info, setInfo] = useState<RootInfo | null>(null);

  // The public GET / banner is a good "is the service healthy?" summary, so the
  // landing page doubles as a status view (and surfaces a boot config error).
  useEffect(() => {
    apiClient
      .get<RootInfo>('/')
      .then((res) => setInfo(res.data))
      .catch(() => setInfo(null));
  }, []);

  const features = [
    { icon: Zap, title: t('home.f1.title'), desc: t('home.f1.desc') },
    { icon: Shield, title: t('home.f2.title'), desc: t('home.f2.desc') },
    {
      icon: Globe,
      title: t('home.f3.title'),
      desc: t('home.f3.desc', { product: PRODUCT_NAME }),
    },
  ];

  return (
    <div className="min-h-screen bg-gradient-to-br from-primary/10 via-background to-background">
      {/* Hero Section */}
      <div className="container mx-auto px-4 py-16">
        <div className="max-w-4xl mx-auto text-center space-y-8">
          {/* Logo/Title */}
          <div className="space-y-4">
            <h1 className="text-6xl font-bold text-primary animate-fadeIn">
              {PRODUCT_NAME}
            </h1>
            <p className="text-xl text-muted-foreground">{t('home.tagline')}</p>
          </div>

          {/* Live service info */}
          {info && (
            <div className="mx-auto max-w-2xl rounded-lg border bg-card p-4 text-left text-sm">
              <div className="mb-2 flex items-center gap-2 font-medium">
                <Server className="h-4 w-4 text-primary" />
                {t('home.statusTitle')}
              </div>
              <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-muted-foreground">
                <dt>{t('home.version')}</dt>
                <dd className="text-foreground">{info.version || '—'}</dd>
                <dt>{t('home.client')}</dt>
                <dd className="text-foreground">{info.clientName || '—'}</dd>
                <dt>{t('home.whatsappWeb')}</dt>
                <dd className="text-foreground">{info.whatsappWebVersion || '—'}</dd>
                <dt>{t('home.docs')}</dt>
                <dd>
                  <a
                    className="text-primary underline"
                    href={info.documentation || '/swagger/index.html'}
                  >
                    {info.documentation || '/swagger/index.html'}
                  </a>
                </dd>
              </dl>
              {info.error && (
                <p className="mt-2 rounded-md bg-destructive/10 p-2 text-destructive">
                  {t('home.attention', { error: info.error })}
                </p>
              )}
            </div>
          )}

          {/* Features */}
          <div className="grid md:grid-cols-3 gap-6 mt-12">
            {features.map(({ icon: Icon, title, desc }) => (
              <div
                key={title}
                className="bg-card border rounded-lg p-6 space-y-3 hover:border-primary transition-colors"
              >
                <div className="w-12 h-12 bg-primary/10 rounded-lg flex items-center justify-center">
                  <Icon className="w-6 h-6 text-primary" />
                </div>
                <h3 className="text-lg font-semibold">{title}</h3>
                <p className="text-sm text-muted-foreground">{desc}</p>
              </div>
            ))}
          </div>

          {/* CTA Button */}
          <div className="mt-12">
            <Button
              size="lg"
              onClick={() => navigate('/manager/login')}
              className="text-lg px-8 py-6 group"
            >
              {t('home.cta')}
              <ArrowRight className="ml-2 w-5 h-5 group-hover:translate-x-1 transition-transform" />
            </Button>
          </div>

          {/* Footer Info */}
          <div className="mt-16 pt-8 border-t">
            <p className="text-sm text-muted-foreground">
              {t('home.footer', { product: PRODUCT_NAME })}
            </p>
            <p className="text-xs text-muted-foreground mt-2">
              {COPYRIGHT_LINE}
            </p>
          </div>
        </div>
      </div>
    </div>
  );
};

export default Home;
