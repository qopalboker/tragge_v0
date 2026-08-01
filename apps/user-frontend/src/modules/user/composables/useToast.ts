import { ref, readonly } from 'vue';

export type ToastType = 'success' | 'error' | 'warning' | 'info';

export interface Toast {
  id: number;
  message: string;
  type: ToastType;
  duration: number;
}

const toasts = ref<Toast[]>([]);
let nextId = 1;

export function useToast() {
  function show(message: string, type: ToastType = 'info', duration = 4000): number {
    const id = nextId++;
    const toast: Toast = { id, message, type, duration };
    toasts.value.push(toast);

    if (duration > 0) {
      setTimeout(() => {
        remove(id);
      }, duration);
    }

    return id;
  }

  function success(message: string, duration = 4000): number {
    return show(message, 'success', duration);
  }

  function error(message: string, duration = 5000): number {
    return show(message, 'error', duration);
  }

  function warning(message: string, duration = 4000): number {
    return show(message, 'warning', duration);
  }

  function info(message: string, duration = 4000): number {
    return show(message, 'info', duration);
  }

  function remove(id: number): void {
    const index = toasts.value.findIndex(t => t.id === id);
    if (index !== -1) {
      toasts.value.splice(index, 1);
    }
  }

  function clear(): void {
    toasts.value = [];
  }

  return {
    toasts: readonly(toasts),
    show,
    success,
    error,
    warning,
    info,
    remove,
    clear,
  };
}
