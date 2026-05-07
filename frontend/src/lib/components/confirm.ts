import ConfirmDialog from './ConfirmDialog.svelte';

export type ConfirmOptions = {
  title: string;
  message: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
};

export function confirm(opts: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    const target = document.createElement('div');
    document.body.appendChild(target);
    const component = new ConfirmDialog({
      target,
      props: {
        ...opts,
        onResult: (ok: boolean) => {
          component.$destroy();
          target.remove();
          resolve(ok);
        },
      },
    });
  });
}
