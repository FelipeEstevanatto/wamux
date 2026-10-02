import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { toast } from 'sonner';
import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
} from '@/components/ui';
import * as instancesApi from '@/services/api/instances';
import type { Instance } from '@/types/instance';
import { useI18n } from '@/i18n/I18nContext';

interface SendMessageModalProps {
  open: boolean;
  onClose: () => void;
  instance: Instance | null;
}

const sendMessageSchema = z.object({
  number: z.string().min(1, 'Número é obrigatório'),
  message: z.string().min(1, 'Mensagem é obrigatória'),
});

type SendMessageFormData = z.infer<typeof sendMessageSchema>;

/**
 * Built on the shared Dialog primitive, so it traps focus, closes on Escape and
 * returns focus to the trigger — no bespoke focus handling needed.
 */
function SendMessageModal({ open, onClose, instance }: SendMessageModalProps) {
  const { t } = useI18n();
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
    reset,
  } = useForm<SendMessageFormData>({
    resolver: zodResolver(sendMessageSchema),
  });

  const onSubmit = async (data: SendMessageFormData) => {
    if (!instance?.apikey) {
      toast.error(t('sendMessage.tokenMissing'));
      return;
    }

    try {
      await instancesApi.sendMessage(instance.apikey, {
        number: data.number,
        text: data.message,
      });
      toast.success(t('sendMessage.success'));
      reset();
      onClose();
    } catch (error) {
      console.error('Send message error:', error);
      toast.error(t('sendMessage.error'));
    }
  };

  const handleClose = () => {
    reset();
    onClose();
  };

  return (
    <Dialog
      open={open && !!instance}
      onOpenChange={(next) => {
        if (!next) handleClose();
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t('sendMessage.title')}
            {instance ? ` — ${instance.instanceName}` : ''}
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="send-number">{t('sendMessage.number')}</Label>
            <Input
              id="send-number"
              type="text"
              placeholder={t('sendMessage.numberPlaceholder')}
              {...register('number')}
            />
            {errors.number && (
              <p className="text-sm text-destructive">{errors.number.message}</p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="send-message">{t('sendMessage.message')}</Label>
            <textarea
              id="send-message"
              rows={4}
              placeholder={t('sendMessage.messagePlaceholder')}
              {...register('message')}
              className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none transition-[color,box-shadow,border-color] placeholder:text-muted-foreground hover:border-ring/50 focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]"
            />
            {errors.message && (
              <p className="text-sm text-destructive">{errors.message.message}</p>
            )}
          </div>

          <DialogFooter className="gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={handleClose}
              disabled={isSubmitting}
            >
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? t('sendMessage.submitting') : t('sendMessage.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export default SendMessageModal;
