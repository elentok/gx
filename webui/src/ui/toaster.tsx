import { Link, Portal, Toast, Toaster, createToaster } from "@chakra-ui/react";

export const toaster = createToaster({ placement: "bottom-end", pauseOnPageIdle: true });

export interface ToastSpec {
  type: "success" | "error" | "warning" | "info";
  title: string;
  link?: { href: string; label: string };
  description?: string;
  /** Showing the same id again updates that toast instead of adding one. */
  id?: string;
  /** Stays until the close button or code removes it. */
  persistent?: boolean;
}

export function showToast({ type, title, link, description, id, persistent }: ToastSpec) {
  // Chakra spreads these over its defaults, so an undefined id or duration would erase them.
  toaster.create({
    ...(id && { id }),
    ...(persistent && { duration: Infinity }),
    type,
    title,
    description: link ? <Link href={link.href}>{link.label}</Link> : description,
  });
}

/** Mount once under the ChakraProvider. */
export function AppToaster() {
  return (
    <Portal>
      <Toaster toaster={toaster} insetInline={{ mdDown: "4" }}>
        {(toast) => (
          <Toast.Root width={{ md: "sm" }}>
            <Toast.Indicator />
            <div>
              {toast.title && <Toast.Title>{toast.title}</Toast.Title>}
              {toast.description && <Toast.Description>{toast.description}</Toast.Description>}
            </div>
            <Toast.CloseTrigger />
          </Toast.Root>
        )}
      </Toaster>
    </Portal>
  );
}
