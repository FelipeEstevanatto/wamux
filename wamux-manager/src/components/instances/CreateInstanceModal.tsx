/**
 * Create Instance Modal Component
 * Modal for creating a new WhatsApp instance
 */

import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Button,
  Input,
  Label,
} from '@/components/ui';
import { Plus, Loader2, ChevronDown, ChevronUp } from 'lucide-react';
import { toast } from 'sonner';
import * as instancesApi from '@/services/api/instances';
import useInstances from '@/hooks/useInstances';
import type { CreateInstancePayload } from '@/types/instance';
import { v4 as uuidv4 } from 'uuid';
import { useI18n } from '@/i18n/I18nContext';

interface CreateInstanceModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Validation schema
const createInstanceSchema = z.object({
  instanceName: z
    .string()
    .min(1, 'instanceSettings.validation.nameRequired')
    .min(3, 'instanceSettings.validation.nameMin')
    .max(50, 'instanceSettings.validation.nameMax')
    .regex(
      /^[a-zA-Z0-9-_]+$/,
      'instanceSettings.validation.namePattern'
    ),
  token: z.string().optional(),
  proxyHost: z.string().optional(),
  proxyPort: z.string().optional(),
  proxyUsername: z.string().optional(),
  proxyPassword: z.string().optional(),
});

type CreateInstanceFormData = z.infer<typeof createInstanceSchema>;

export default function CreateInstanceModal({
  open,
  onOpenChange,
}: CreateInstanceModalProps) {
  const { t } = useI18n();
  const [isLoading, setIsLoading] = useState(false);
  const [showProxyConfig, setShowProxyConfig] = useState(false);
  const { addInstance, fetchInstances } = useInstances();

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<CreateInstanceFormData>({
    resolver: zodResolver(createInstanceSchema),
    defaultValues: {
      instanceName: '',
      token: '',
      proxyHost: '',
      proxyPort: '',
      proxyUsername: '',
      proxyPassword: '',
    },
  });

  const onSubmit = async (data: CreateInstanceFormData) => {
    setIsLoading(true);

    try {
      const payload: CreateInstancePayload = {
        name: data.instanceName,
        token: data.token || uuidv4(), // Generate UUID if not provided
      };

      // Add proxy configuration if provided
      if (data.proxyHost && data.proxyPort) {
        payload.proxy = {
          host: data.proxyHost,
          port: data.proxyPort,
          username: data.proxyUsername,
          password: data.proxyPassword,
        };
      }

      const newInstance = await instancesApi.createInstance(payload);

      // Add to store
      addInstance(newInstance);

      toast.success(t('instanceSettings.create.success'), {
        description: t('instanceSettings.create.successDescription', {
          name: data.instanceName,
        }),
      });

      // Refresh instances list
      await fetchInstances();

      // Close modal and reset form
      onOpenChange(false);
      reset();
    } catch (error) {
      console.error('Erro ao criar instância:', error);
      toast.error(
        error instanceof Error
          ? error.message
          : t('instanceSettings.create.error')
      );
    } finally {
      setIsLoading(false);
    }
  };

  const handleClose = () => {
    if (!isLoading) {
      onOpenChange(false);
      reset();
      setShowProxyConfig(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Plus className="h-5 w-5 text-primary" />
            {t('instanceSettings.create.title')}
          </DialogTitle>
          <DialogDescription className="text-sidebar-foreground/70">
            {t('instanceSettings.create.description')}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          {/* Instance Name */}
          <div className="space-y-2">
            <Label htmlFor="instanceName">
              {t('instanceSettings.field.instanceName')} <span className="text-red-500">*</span>
            </Label>
            <Input
              id="instanceName"
              type="text"
              placeholder={t('instanceSettings.create.instanceNamePlaceholder')}
              disabled={isLoading}
              {...register('instanceName')}
              className="bg-sidebar border-sidebar-border text-sidebar-foreground placeholder:text-sidebar-foreground/50"
            />
            {errors.instanceName && (
              <p className="text-destructive text-sm">
                {t(errors.instanceName.message ?? '')}
              </p>
            )}
            <p className="text-xs text-sidebar-foreground/60">
              {t('instanceSettings.create.instanceNameHelp')}
            </p>
          </div>

          {/* Token (Optional) */}
          <div className="space-y-2">
            <Label htmlFor="token">{t('instanceSettings.field.token')}</Label>
            <Input
              id="token"
              type="text"
              placeholder={t('instanceSettings.create.tokenPlaceholder')}
              disabled={isLoading}
              {...register('token')}
              className="bg-sidebar border-sidebar-border text-sidebar-foreground placeholder:text-sidebar-foreground/50"
            />
            {errors.token && (
              <p className="text-destructive text-sm">
                {t(errors.token.message ?? '')}
              </p>
            )}
            <p className="text-xs text-sidebar-foreground/60">
              {t('instanceSettings.create.tokenHelp')}
            </p>
          </div>

          {/* Proxy Configuration (Collapsible) */}
          <div className="space-y-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => setShowProxyConfig(!showProxyConfig)}
              disabled={isLoading}
              className="w-full justify-between bg-sidebar border-sidebar-border text-sidebar-foreground hover:bg-sidebar-accent"
            >
              <span>{t('instanceSettings.create.proxyToggle')}</span>
              {showProxyConfig ? (
                <ChevronUp className="h-4 w-4" />
              ) : (
                <ChevronDown className="h-4 w-4" />
              )}
            </Button>

            {showProxyConfig && (
              <div className="space-y-4 pt-2 border-t border-sidebar-border">
                {/* Proxy Host */}
                <div className="space-y-2">
                  <Label htmlFor="proxyHost">{t('instanceSettings.field.proxyHost')}</Label>
                  <Input
                    id="proxyHost"
                    type="text"
                    placeholder={t('instanceSettings.create.proxyHostPlaceholder')}
                    disabled={isLoading}
                    {...register('proxyHost')}
                    className="bg-sidebar border-sidebar-border text-sidebar-foreground placeholder:text-sidebar-foreground/50"
                  />
                  {errors.proxyHost && (
                    <p className="text-destructive text-sm">
                      {t(errors.proxyHost.message ?? '')}
                    </p>
                  )}
                </div>

                {/* Proxy Port */}
                <div className="space-y-2">
                  <Label htmlFor="proxyPort">{t('instanceSettings.field.proxyPort')}</Label>
                  <Input
                    id="proxyPort"
                    type="text"
                    placeholder={t('instanceSettings.create.proxyPortPlaceholder')}
                    disabled={isLoading}
                    {...register('proxyPort')}
                    className="bg-sidebar border-sidebar-border text-sidebar-foreground placeholder:text-sidebar-foreground/50"
                  />
                  {errors.proxyPort && (
                    <p className="text-destructive text-sm">
                      {t(errors.proxyPort.message ?? '')}
                    </p>
                  )}
                </div>

                {/* Proxy Username */}
                <div className="space-y-2">
                  <Label htmlFor="proxyUsername">{t('instanceSettings.field.proxyUsername')}</Label>
                  <Input
                    id="proxyUsername"
                    type="text"
                    placeholder={t('instanceSettings.create.proxyUsernamePlaceholder')}
                    disabled={isLoading}
                    {...register('proxyUsername')}
                    className="bg-sidebar border-sidebar-border text-sidebar-foreground placeholder:text-sidebar-foreground/50"
                  />
                  {errors.proxyUsername && (
                    <p className="text-destructive text-sm">
                      {t(errors.proxyUsername.message ?? '')}
                    </p>
                  )}
                </div>

                {/* Proxy Password */}
                <div className="space-y-2">
                  <Label htmlFor="proxyPassword">{t('instanceSettings.field.proxyPassword')}</Label>
                  <Input
                    id="proxyPassword"
                    type="password"
                    placeholder={t('instanceSettings.create.proxyPasswordPlaceholder')}
                    disabled={isLoading}
                    {...register('proxyPassword')}
                    className="bg-sidebar border-sidebar-border text-sidebar-foreground placeholder:text-sidebar-foreground/50"
                  />
                  {errors.proxyPassword && (
                    <p className="text-destructive text-sm">
                      {t(errors.proxyPassword.message ?? '')}
                    </p>
                  )}
                </div>
              </div>
            )}
          </div>

          <DialogFooter className="flex gap-2 sm:gap-0">
            <Button
              type="button"
              variant="outline"
              onClick={handleClose}
              disabled={isLoading}
              className="bg-sidebar border-sidebar-border text-sidebar-foreground hover:bg-sidebar-accent"
            >
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={isLoading}>
              {isLoading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  {t('instanceSettings.create.submitting')}
                </>
              ) : (
                <>
                  <Plus className="mr-2 h-4 w-4" />
                  {t('instanceSettings.create.submit')}
                </>
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
