<script setup lang="ts">
import { CheckIcon } from '@lucide/vue';

import type { CheckboxRootEmits, CheckboxRootProps } from "reka-ui"
import type { HTMLAttributes } from "vue"
import { computed } from "vue"
import { reactiveOmit } from "@vueuse/core"
import { CheckboxIndicator, CheckboxRoot } from "reka-ui"
import { cn } from "@/lib/utils"

const props = defineProps<CheckboxRootProps & {
  class?: HTMLAttributes["class"]
  /** shadcn 兼容：与 modelValue 二选一，对应 v-model:checked */
  checked?: boolean | 'indeterminate' | null
}>()

const emits = defineEmits<CheckboxRootEmits>()

const model = computed({
  get: () => (props.modelValue !== undefined ? props.modelValue : props.checked),
  set: (value) => {
    emits('update:modelValue', value as boolean | 'indeterminate')
    ;(emits as unknown as (event: 'update:checked', value: boolean | 'indeterminate') => void)(
      'update:checked',
      value as boolean | 'indeterminate',
    )
  },
})

const delegatedProps = reactiveOmit(props, "class", "checked", "modelValue")
</script>

<template>
  <CheckboxRoot
    v-slot="slotProps"
    v-model="model"
    data-slot="checkbox"
    v-bind="delegatedProps"
    :class="cn('border-input dark:bg-input/30 data-checked:bg-primary data-checked:text-primary-foreground dark:data-checked:bg-primary data-checked:border-primary aria-invalid:aria-checked:border-primary aria-invalid:border-destructive dark:aria-invalid:border-destructive/50 focus-visible:border-ring focus-visible:ring-ring/50 aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 flex size-4 items-center justify-center rounded-[4px] border transition-colors group-has-disabled/field:opacity-50 focus-visible:ring-3 aria-invalid:ring-3 peer relative shrink-0 outline-none after:absolute after:-inset-x-3 after:-inset-y-2 disabled:cursor-not-allowed disabled:opacity-50', props.class)"
  >
    <CheckboxIndicator
      data-slot="checkbox-indicator"
      class="[&>svg]:size-3.5 grid place-content-center text-current transition-none"
    >
      <slot v-bind="slotProps">
        <CheckIcon />
      </slot>
    </CheckboxIndicator>
  </CheckboxRoot>
</template>
