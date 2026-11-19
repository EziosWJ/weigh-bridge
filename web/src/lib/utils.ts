import { clsx, type ClassValue } from "clsx";
import { extendTailwindMerge } from "tailwind-merge";

// 与 tailwind.config.ts 中的字号令牌保持一致，使字号和文字颜色可以共存。
const mergeClasses = extendTailwindMerge({
  extend: {
    classGroups: {
      "font-size": [
        { text: ["page-title", "section-title", "card-title", "body", "form-label", "body-secondary", "helper", "table-cell"] },
      ],
    },
  },
});

export function cn(...inputs: ClassValue[]) {
  return mergeClasses(clsx(inputs));
}

