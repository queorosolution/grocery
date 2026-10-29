import clsx from "clsx";
import { ImageOff } from "lucide-react";
import { safeUrl } from "@/lib/safeUrl";

interface ThumbProps {
  src: string | null | undefined;
  /** Leave empty when the name is written next to it. */
  alt?: string;
  className?: string;
}

/** A square picture. A missing or unsafe address shows a quiet placeholder instead. */
export function Thumb({ src, alt = "", className }: ThumbProps) {
  const url = safeUrl(src);
  return url ? (
    // Images are resized at upload (docs/08 section 1), so next/image has nothing to add.
    // eslint-disable-next-line @next/next/no-img-element
    <img src={url} alt={alt} loading="lazy" decoding="async" className={clsx("aspect-square rounded-input bg-surface object-cover", className)} />
  ) : (
    <div aria-hidden="true" className={clsx("flex aspect-square items-center justify-center rounded-input bg-surface text-muted", className)}>
      <ImageOff size={20} />
    </div>
  );
}
