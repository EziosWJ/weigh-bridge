import { RotateCcw, Save } from "lucide-react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { Field } from "@/components/common/field";
import { FormSection } from "@/components/common/form-section";
import { PageHeader } from "@/components/common/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";

const formSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "请输入配置名称")
    .max(30, "配置名称不能超过 30 个字符"),
  code: z
    .string()
    .trim()
    .min(1, "请输入配置编码")
    .max(40, "配置编码不能超过 40 个字符")
    .regex(/^[a-zA-Z][a-zA-Z0-9_]*$/, "编码需以字母开头，仅支持字母、数字和下划线"),
  type: z.enum(["internal", "business", "security"], {
    errorMap: () => ({ message: "请选择有效的配置类型" }),
  }),
  owner: z
    .string()
    .trim()
    .min(1, "请输入负责人")
    .max(20, "负责人不能超过 20 个字符"),
  ownerEmail: z
    .string()
    .trim()
    .min(1, "请输入负责人邮箱")
    .email("请输入有效的邮箱地址"),
  enabled: z.boolean(),
  description: z
    .string()
    .trim()
    .max(200, "配置说明不能超过 200 个字符")
    .optional(),
});

type FormValues = z.infer<typeof formSchema>;

const defaultValues: FormValues = {
  name: "",
  code: "",
  type: "internal",
  owner: "",
  ownerEmail: "",
  enabled: true,
  description: "",
};

function sleep(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

export function FormExamplePage() {
  const {
    control,
    register,
    handleSubmit,
    reset,
    formState: { errors, isSubmitting, isSubmitSuccessful },
  } = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues,
    mode: "onBlur",
  });

  const onSubmit = async () => {
    await sleep(600);
  };

  const handleReset = () => {
    reset(defaultValues);
  };

  return (
    <>
      <PageHeader
        title="标准表单 Demo"
        description="表单页示例，展示分组表单、Zod 校验和 React Hook Form 提交状态。"
      />

      <form
        className="max-w-[1080px] space-y-6"
        onSubmit={handleSubmit(onSubmit)}
        noValidate
      >
        <FormSection
          title="基础信息"
          description="用于演示后台配置类表单的常见字段。"
        >
          <Field
            label="配置名称"
            htmlFor="name"
            required
            error={errors.name?.message}
          >
            <Input
              id="name"
              placeholder="请输入配置名称"
              disabled={isSubmitting}
              {...register("name")}
            />
          </Field>

          <Field
            label="配置编码"
            htmlFor="code"
            required
            error={errors.code?.message}
            help="以字母开头，支持字母、数字和下划线。"
          >
            <Input
              id="code"
              placeholder="例如 system_notice"
              disabled={isSubmitting}
              {...register("code")}
            />
          </Field>

          <Field
            label="配置类型"
            htmlFor="type"
            required
            error={errors.type?.message}
          >
            <Select id="type" disabled={isSubmitting} {...register("type")}>
              <option value="internal">内部配置</option>
              <option value="business">业务配置</option>
              <option value="security">安全配置</option>
            </Select>
          </Field>

          <Field
            label="负责人"
            htmlFor="owner"
            required
            error={errors.owner?.message}
          >
            <Input
              id="owner"
              placeholder="请输入负责人"
              disabled={isSubmitting}
              {...register("owner")}
            />
          </Field>

          <div className="md:col-span-2">
            <Field
              label="负责人邮箱"
              htmlFor="ownerEmail"
              required
              error={errors.ownerEmail?.message}
            >
              <Input
                id="ownerEmail"
                type="email"
                inputMode="email"
                autoComplete="email"
                placeholder="name@example.com"
                disabled={isSubmitting}
                {...register("ownerEmail")}
              />
            </Field>
          </div>
        </FormSection>

        <FormSection
          title="发布设置"
          description="演示自定义开关组件和文本域的表单绑定。"
        >
          <div className="md:col-span-2">
            <div className="flex items-center justify-between rounded-lg border border-border px-4 py-3">
              <div>
                <div className="font-medium text-text-primary">启用配置</div>
                <div className="text-sm text-text-tertiary">
                  关闭后该配置不会在业务中生效。
                </div>
              </div>
              <Controller
                control={control}
                name="enabled"
                render={({ field }) => (
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={isSubmitting}
                  />
                )}
              />
            </div>
          </div>

          <div className="md:col-span-2">
            <Field
              label="配置说明"
              htmlFor="description"
              error={errors.description?.message}
              help="最多 200 个字符。"
            >
              <Textarea
                id="description"
                placeholder="请输入说明"
                disabled={isSubmitting}
                {...register("description")}
              />
            </Field>
          </div>
        </FormSection>

        <div className="flex flex-col gap-3 rounded-admin border border-border bg-surface px-5 py-4 shadow-admin md:flex-row md:items-center md:justify-between">
          <p className="text-sm text-text-tertiary">
            {isSubmitSuccessful
              ? "提交成功，当前为静态成功提示，未请求后端接口。"
              : "提交后仅进行本地状态反馈，不请求后端接口。"}
          </p>
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              onClick={handleReset}
              disabled={isSubmitting}
            >
              <RotateCcw className="h-4 w-4" aria-hidden />
              重置
            </Button>
            <Button type="submit" variant="primary" disabled={isSubmitting}>
              <Save className="h-4 w-4" aria-hidden />
              {isSubmitting ? "提交中..." : "提交"}
            </Button>
          </div>
        </div>
      </form>
    </>
  );
}
