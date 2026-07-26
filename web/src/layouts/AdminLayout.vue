<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import { Separator } from '@/components/ui/separator'
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from '@/components/ui/sidebar'

const route = useRoute()

const pageTitle = computed(() => (route.meta.title as string) || '管理后台')
const pageDescription = computed(() => route.meta.description as string | undefined)
</script>

<template>
  <SidebarProvider>
    <AppSidebar />
    <SidebarInset>
      <header
        class="flex shrink-0 items-center gap-2 border-b px-4 py-3 md:px-6"
        :class="pageDescription ? 'min-h-[4.25rem]' : 'min-h-14'"
      >
        <SidebarTrigger class="-ml-1" />
        <Separator orientation="vertical" class="mr-1 hidden !h-6 sm:block" />
        <div class="min-w-0 flex-1">
          <h1 class="truncate text-base font-semibold tracking-tight">{{ pageTitle }}</h1>
          <p v-if="pageDescription" class="mt-0.5 truncate text-sm text-muted-foreground">
            {{ pageDescription }}
          </p>
        </div>
      </header>
      <div class="flex flex-1 flex-col p-4 md:p-6">
        <RouterView />
      </div>
    </SidebarInset>
  </SidebarProvider>
</template>
