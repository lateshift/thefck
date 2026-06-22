<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import {
  FlexRender,
  createColumnHelper,
  getCoreRowModel,
  getExpandedRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useVueTable,
} from '@tanstack/vue-table'

const pageSize = 200
const records = ref([])
const loading = ref(true)
const error = ref('')
const search = ref('')
const duplicateFilter = ref('duplicates')
const sorting = ref([{ id: 'absolute_path', desc: false }])
const pagination = ref({ pageIndex: 0, pageSize })
const expanded = ref({})

const columnHelper = createColumnHelper()
const columns = [
  columnHelper.accessor('duplicate', {
    header: 'Duplicate',
    cell: (info) => statusLabel(info.getValue()),
    sortingFn: 'basic',
  }),
  columnHelper.accessor('filedate', {
    header: 'File date',
    cell: (info) => formatDate(info.getValue()),
    sortingFn: 'datetime',
  }),
  columnHelper.accessor('filesize', {
    header: 'File size',
    cell: (info) => formatBytes(info.getValue()),
    sortingFn: 'basic',
  }),
  columnHelper.accessor('filename', {
    header: 'Filename',
    cell: (info) => info.getValue(),
  }),
  columnHelper.accessor('absolute_path', {
    header: 'Absolute path',
    cell: (info) => info.getValue(),
  }),
  columnHelper.accessor('checksum', {
    header: 'Checksum',
    cell: (info) => info.getValue(),
  }),
]

// Filters are intentionally local: the index is already loaded and TanStack can
// sort and paginate the filtered records without another API request.
const filteredRecords = computed(() => {
  const term = search.value.trim().toLowerCase()

  return records.value.filter((record) => {
    if (duplicateFilter.value === 'duplicates' && !record.duplicate) {
      return false
    }
    if (duplicateFilter.value === 'unique' && record.duplicate) {
      return false
    }
    if (!term) {
      return true
    }

    const values = [
      statusLabel(record.duplicate),
      formatDate(record.filedate),
      String(record.filesize ?? ''),
      formatBytes(record.filesize),
      record.filename,
      record.absolute_path,
      record.checksum,
    ]
    return values.some((value) => String(value ?? '').toLowerCase().includes(term))
  })
})

const duplicateGroups = computed(() => {
  const groups = new Map()
  for (const record of records.value) {
    const key = duplicateGroupKey(record)
    if (!key) {
      continue
    }
    if (!groups.has(key)) {
      groups.set(key, [])
    }
    groups.get(key).push(record)
  }

  for (const group of groups.values()) {
    group.sort(compareRecordsByPath)
  }
  return groups
})

const tableRecords = computed(() =>
  filteredRecords.value.map((record) => {
    const group = duplicateGroups.value.get(duplicateGroupKey(record)) ?? []
    const subRows = record.duplicate
      ? group
          .filter((match) => match.absolute_path !== record.absolute_path)
          .map((match) => ({
            ...match,
            subRows: [],
          }))
      : []

    return {
      ...record,
      subRows,
    }
  }),
)

const duplicateCount = computed(() => records.value.filter((record) => record.duplicate).length)
const activeFilters = computed(() => {
  const filters = [
    {
      key: 'duplicate',
      label: duplicateFilter.value === 'duplicates' ? 'Duplicate' : 'Unique',
      value: duplicateFilter.value === 'all' ? 'All files' : 'On',
    },
  ]

  if (search.value.trim()) {
    filters.push({ key: 'search', label: 'Search', value: search.value.trim() })
  }

  return filters
})

const table = useVueTable({
  get data() {
    return tableRecords.value
  },
  columns,
  state: {
    get sorting() {
      return sorting.value
    },
    get pagination() {
      return pagination.value
    },
    get expanded() {
      return expanded.value
    },
  },
  onSortingChange: (updaterOrValue) => {
    sorting.value =
      typeof updaterOrValue === 'function' ? updaterOrValue(sorting.value) : updaterOrValue
  },
  onPaginationChange: (updaterOrValue) => {
    pagination.value =
      typeof updaterOrValue === 'function' ? updaterOrValue(pagination.value) : updaterOrValue
  },
  onExpandedChange: (updaterOrValue) => {
    expanded.value =
      typeof updaterOrValue === 'function' ? updaterOrValue(expanded.value) : updaterOrValue
  },
  getRowId: (record, index, parent) => {
    const path = record.absolute_path || String(index)
    return parent ? `${parent.id}::${path}` : path
  },
  getSubRows: (record) => record.subRows,
  getRowCanExpand: (row) => row.depth === 0 && row.original.subRows.length > 0,
  getCoreRowModel: getCoreRowModel(),
  getExpandedRowModel: getExpandedRowModel(),
  getSortedRowModel: getSortedRowModel(),
  getPaginationRowModel: getPaginationRowModel(),
  paginateExpandedRows: false,
})

const pageRows = computed(() => table.getRowModel().rows)
const pageStart = computed(() =>
  filteredRecords.value.length === 0 ? 0 : pagination.value.pageIndex * pageSize + 1,
)
const pageEnd = computed(() =>
  Math.min((pagination.value.pageIndex + 1) * pageSize, filteredRecords.value.length),
)

onMounted(fetchFiles)

watch([search, duplicateFilter], () => {
  expanded.value = {}
  table.setPageIndex(0)
})

async function fetchFiles() {
  loading.value = true
  error.value = ''
  try {
    const response = await fetch('/api/files')
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }
    const payload = await response.json()
    records.value = Array.isArray(payload.files) ? payload.files : []
    expanded.value = {}
    table.setPageIndex(0)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'Unable to load index'
  } finally {
    loading.value = false
  }
}

function statusLabel(value) {
  return value ? 'Duplicate' : 'Unique'
}

function duplicateGroupKey(record) {
  if (!record?.checksum || record.filesize === undefined || record.filesize === null) {
    return ''
  }
  return `${record.filesize}:${record.checksum}`
}

function compareRecordsByPath(left, right) {
  return String(left.absolute_path ?? '').localeCompare(String(right.absolute_path ?? ''))
}

function toggleDuplicateRow(row) {
  if (row.getCanExpand()) {
    row.toggleExpanded()
  }
}

function formatBytes(value) {
  const size = Number(value)
  if (!Number.isFinite(size)) {
    return '0 B'
  }
  if (size < 1024) {
    return `${size} B`
  }

  const units = ['KB', 'MB', 'GB', 'TB']
  let scaled = size / 1024
  let unitIndex = 0
  while (scaled >= 1024 && unitIndex < units.length - 1) {
    scaled /= 1024
    unitIndex += 1
  }
  return `${scaled.toFixed(scaled >= 10 ? 1 : 2)} ${units[unitIndex]}`
}

function formatDate(value) {
  if (!value || String(value).startsWith('0001-')) {
    return 'Unknown'
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(new Date(value))
}

function sortLabel(direction) {
  if (direction === 'asc') {
    return 'ASC'
  }
  if (direction === 'desc') {
    return 'DESC'
  }
  return ''
}
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <div>
        <h1>Indexed files</h1>
        <p>{{ filteredRecords.length }} / {{ records.length }} files · {{ duplicateCount }} duplicates</p>
      </div>
      <div class="actions">
        <div class="segmented-control" aria-label="Duplicate filter">
          <button
            type="button"
            :class="{ active: duplicateFilter === 'duplicates' }"
            @click="duplicateFilter = 'duplicates'"
          >
            Duplicates
          </button>
          <button
            type="button"
            :class="{ active: duplicateFilter === 'unique' }"
            @click="duplicateFilter = 'unique'"
          >
            Unique
          </button>
          <button type="button" :class="{ active: duplicateFilter === 'all' }" @click="duplicateFilter = 'all'">
            All
          </button>
        </div>
        <input v-model="search" type="search" placeholder="Search index" aria-label="Search index" />
        <button type="button" :disabled="loading" @click="fetchFiles">
          {{ loading ? 'Loading' : 'Refresh' }}
        </button>
      </div>
    </header>

    <main class="table-panel">
      <div v-if="error" class="error">{{ error }}</div>
      <div class="filter-bar">
        <div class="filter-chips">
          <span v-for="filter in activeFilters" :key="filter.key" class="filter-chip">
            {{ filter.label }}: {{ filter.value }}
          </span>
        </div>
        <div class="pagination-summary">
          {{ pageStart }}-{{ pageEnd }} of {{ filteredRecords.length }} · {{ pageSize }} per page
        </div>
      </div>
      <div class="table-scroll">
        <table>
          <thead>
            <tr v-for="headerGroup in table.getHeaderGroups()" :key="headerGroup.id">
              <th v-for="header in headerGroup.headers" :key="header.id" :colspan="header.colSpan">
                <button
                  v-if="!header.isPlaceholder"
                  class="header-button"
                  type="button"
                  @click="header.column.getToggleSortingHandler()?.($event)"
                >
                  <FlexRender :render="header.column.columnDef.header" :props="header.getContext()" />
                  <span>{{ sortLabel(header.column.getIsSorted()) }}</span>
                </button>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td :colspan="columns.length" class="empty-row">Loading</td>
            </tr>
            <tr v-else-if="pageRows.length === 0">
              <td :colspan="columns.length" class="empty-row">No files</td>
            </tr>
            <tr
              v-for="row in pageRows"
              v-else
              :key="row.id"
              :aria-expanded="row.getCanExpand() ? row.getIsExpanded() : undefined"
              :class="{
                'duplicate-row': row.original.duplicate,
                'duplicate-subrow': row.depth > 0,
                'can-expand': row.getCanExpand(),
                expanded: row.getIsExpanded(),
              }"
              :tabindex="row.getCanExpand() ? 0 : undefined"
              @click="toggleDuplicateRow(row)"
              @keydown.enter="toggleDuplicateRow(row)"
              @keydown.space.prevent="toggleDuplicateRow(row)"
            >
              <td
                v-for="cell in row.getVisibleCells()"
                :key="cell.id"
                :class="['cell', `cell-${cell.column.id}`]"
              >
                <div v-if="cell.column.id === 'duplicate'" class="duplicate-cell">
                  <button
                    v-if="row.getCanExpand()"
                    type="button"
                    class="row-toggle"
                    :aria-expanded="row.getIsExpanded()"
                    :aria-label="row.getIsExpanded() ? 'Hide duplicate matches' : 'Show duplicate matches'"
                    :title="row.getIsExpanded() ? 'Hide duplicate matches' : 'Show duplicate matches'"
                    @click.stop="row.toggleExpanded()"
                  >
                    {{ row.getIsExpanded() ? '-' : '+' }}
                  </button>
                  <span v-else class="row-toggle-spacer"></span>
                  <span class="status-pill">
                    <FlexRender :render="cell.column.columnDef.cell" :props="cell.getContext()" />
                  </span>
                  <span v-if="row.getCanExpand()" class="match-count">
                    {{ row.original.subRows.length }} {{ row.original.subRows.length === 1 ? 'match' : 'matches' }}
                  </span>
                </div>
                <FlexRender v-else :render="cell.column.columnDef.cell" :props="cell.getContext()" />
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <nav class="pagination-controls" aria-label="Table pagination">
        <button type="button" :disabled="!table.getCanPreviousPage()" @click="table.setPageIndex(0)">First</button>
        <button type="button" :disabled="!table.getCanPreviousPage()" @click="table.previousPage()">Previous</button>
        <span>Page {{ pagination.pageIndex + 1 }} of {{ table.getPageCount() || 1 }}</span>
        <button type="button" :disabled="!table.getCanNextPage()" @click="table.nextPage()">Next</button>
        <button
          type="button"
          :disabled="!table.getCanNextPage()"
          @click="table.setPageIndex(table.getPageCount() - 1)"
        >
          Last
        </button>
      </nav>
    </main>
  </div>
</template>
