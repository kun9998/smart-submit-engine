<script lang="ts" setup>
import {
  CircleCheckIcon,
  InfoIcon,
  TriangleAlertIcon,
  OctagonXIcon,
  Loader2Icon,
  XIcon,
} from '@lucide/vue'
import type { ToasterProps } from 'vue-sonner'
import { Toaster as Sonner } from 'vue-sonner'
import { cn } from '@/lib/utils'

const props = withDefaults(defineProps<ToasterProps>(), {
  position: 'top-right',
  duration: 5000,
  closeButton: true,
  expand: true,
  richColors: true,
})
</script>

<template>
  <Sonner
    theme="light"
    :class="cn('toaster group', props.class)"
    :style="{
      '--normal-bg': 'var(--popover)',
      '--normal-text': 'var(--popover-foreground)',
      '--normal-border': 'var(--border)',
      '--border-radius': 'var(--radius)',
    }"
    :position="props.position"
    :duration="props.duration"
    :close-button="props.closeButton"
    :expand="props.expand"
    :rich-colors="props.richColors"
    :toast-options="{
      ...props.toastOptions,
      duration: props.duration,
      classes: {
        ...props.toastOptions?.classes,
        toast: 'rounded-2xl max-w-[420px] break-words',
      },
    }"
  >
    <template #success-icon>
      <CircleCheckIcon class="size-4" />
    </template>
    <template #info-icon>
      <InfoIcon class="size-4" />
    </template>
    <template #warning-icon>
      <TriangleAlertIcon class="size-4" />
    </template>
    <template #error-icon>
      <OctagonXIcon class="size-4" />
    </template>
    <template #loading-icon>
      <div>
        <Loader2Icon class="size-4 animate-spin" />
      </div>
    </template>
    <template #close-icon>
      <XIcon class="size-4" />
    </template>
  </Sonner>
</template>
