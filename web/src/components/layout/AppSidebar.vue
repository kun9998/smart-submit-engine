<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Activity,
  BookOpen,
  Bot,
  ChevronsUpDown,
  Gauge,
  LayoutDashboard,
  LogOut,
  Package,
  Rocket,
  ScrollText,
  Settings,
  Settings2,
  User,
} from '@lucide/vue'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from '@/components/ui/sidebar'
import { authApi, clearSession, getUsername } from '@/api/auth'
import { systemApi } from '@/api/system'
import { adminNavGroups, adminProductName } from '@/lib/admin-nav'

const route = useRoute()
const router = useRouter()
const username = computed(() => getUsername() || '管理员')
const initials = computed(() => username.value.slice(0, 2).toUpperCase())
const productVersion = ref('')

const navIconByPath: Record<string, typeof Gauge> = {
  '/': Gauge,
  '/platforms': LayoutDashboard,
  '/huoyuan': Package,
  '/logs': ScrollText,
  '/docs': BookOpen,
  '/settings': Settings,
  '/monitor': Activity,
  '/ops': Bot,
  '/upgrade': Rocket,
}

const navGroups = adminNavGroups.map((group) => ({
  label: group.label,
  items: group.items.map((item) => ({
    title: item.title,
    to: item.path,
    icon: navIconByPath[item.path] ?? Settings,
  })),
}))

function isNavActive(path: string) {
  if (path === '/') return route.path === '/'
  return route.path === path || route.path.startsWith(`${path}/`)
}

onMounted(async () => {
  try {
    const res = await systemApi.info()
    if (res.code === 1 && res.data?.product_version) {
      productVersion.value = res.data.product_version
    }
  } catch {
    // 未登录时忽略
  }
})

async function logout() {
  await authApi.logout()
  clearSession()
  await router.push('/login')
}
</script>

<template>
  <Sidebar collapsible="icon">
    <SidebarHeader>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton size="lg" class="pointer-events-none">
            <div class="flex aspect-square size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
              <Settings2 class="size-4" />
            </div>
            <div class="grid flex-1 text-left text-sm leading-tight">
              <span class="truncate font-semibold">{{ adminProductName }}</span>
              <span class="truncate text-xs text-muted-foreground">
                管理后台<span v-if="productVersion"> · {{ productVersion }}</span>
              </span>
            </div>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarHeader>

    <SidebarContent>
      <SidebarGroup v-for="group in navGroups" :key="group.label">
        <SidebarGroupLabel>{{ group.label }}</SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem v-for="item in group.items" :key="item.to">
              <SidebarMenuButton as-child :is-active="isNavActive(item.to)" :tooltip="item.title">
                <RouterLink :to="item.to">
                  <component :is="item.icon" />
                  <span>{{ item.title }}</span>
                </RouterLink>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </SidebarContent>

    <SidebarFooter>
      <SidebarMenu>
        <SidebarMenuItem>
          <DropdownMenu>
            <DropdownMenuTrigger as-child>
              <SidebarMenuButton size="lg" class="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground">
                <Avatar class="size-8 rounded-lg">
                  <AvatarFallback class="rounded-lg">{{ initials }}</AvatarFallback>
                </Avatar>
                <div class="grid flex-1 text-left text-sm leading-tight">
                  <span class="truncate font-semibold">{{ username }}</span>
                  <span class="truncate text-xs text-muted-foreground">管理员</span>
                </div>
                <ChevronsUpDown class="ml-auto size-4" />
              </SidebarMenuButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent class="w-[--reka-dropdown-menu-trigger-width] min-w-56 rounded-lg" side="bottom" align="end" :side-offset="4">
              <DropdownMenuLabel class="p-0 font-normal">
                <div class="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
                  <Avatar class="size-8 rounded-lg">
                    <AvatarFallback class="rounded-lg">{{ initials }}</AvatarFallback>
                  </Avatar>
                  <div class="grid flex-1 text-left text-sm leading-tight">
                    <span class="truncate font-semibold">{{ username }}</span>
                    <span class="truncate text-xs text-muted-foreground">管理员</span>
                  </div>
                </div>
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem @click="router.push('/profile')">
                <User />
                个人中心
              </DropdownMenuItem>
              <DropdownMenuItem @click="logout">
                <LogOut />
                退出登录
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarFooter>
    <SidebarRail />
  </Sidebar>
</template>
