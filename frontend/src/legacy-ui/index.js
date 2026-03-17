/* eslint-disable vue/one-component-per-file */
import {
  Teleport,
  computed,
  defineComponent,
  h,
  nextTick,
  ref,
} from 'vue';

function normalizeClass(value) {
  if (!value) {
    return [];
  }

  if (Array.isArray(value)) {
    return value.flatMap(normalizeClass);
  }

  if (typeof value === 'object') {
    return Object.entries(value)
      .filter(([, enabled]) => Boolean(enabled))
      .map(([key]) => key);
  }

  return [value];
}

function toKebabCase(value = '') {
  return String(value)
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
    .replace(/_/g, '-')
    .toLowerCase();
}

function renderDefaultSlot(slots, args) {
  return slots.default ? slots.default(args) : [];
}

const BIcon = defineComponent({
  name: 'BIcon',
  props: {
    icon: { type: String, default: '' },
    size: { type: String, default: '' },
    type: { type: String, default: '' },
  },
  render() {
    return h(
      'span',
      {
        class: [
          'icon',
          ...normalizeClass(this.size),
          ...normalizeClass(this.type),
        ],
      },
      [
        h('i', {
          class: ['mdi', `mdi-${toKebabCase(this.icon)}`],
          'aria-hidden': 'true',
        }),
      ],
    );
  },
});

const BTag = defineComponent({
  name: 'BTag',
  emits: ['close'],
  props: {
    type: { type: String, default: '' },
    size: { type: String, default: '' },
    closable: { type: Boolean, default: false },
  },
  render() {
    return h(
      'span',
      {
        class: ['tag', ...normalizeClass(this.type), ...normalizeClass(this.size)],
      },
      [
        ...renderDefaultSlot(this.$slots),
        this.closable
          ? h(
            'button',
            {
              type: 'button',
              class: 'tag-close-button',
              onClick: (event) => {
                event.stopPropagation();
                this.$emit('close');
              },
            },
            '×',
          )
          : null,
      ],
    );
  },
});

const BTaglist = defineComponent({
  name: 'BTaglist',
  render() {
    return h('div', { class: 'tags' }, renderDefaultSlot(this.$slots));
  },
});

const BButton = defineComponent({
  name: 'BButton',
  emits: ['click'],
  props: {
    type: { type: String, default: '' },
    nativeType: { type: String, default: 'button' },
    iconLeft: { type: String, default: '' },
    expanded: { type: Boolean, default: false },
    disabled: { type: Boolean, default: false },
    loading: { type: Boolean, default: false },
  },
  render() {
    return h(
      'button',
      {
        ...this.$attrs,
        type: this.nativeType || 'button',
        disabled: this.disabled || this.loading,
        class: [
          'button',
          ...normalizeClass(this.type),
          this.expanded ? 'is-fullwidth' : null,
          this.loading ? 'is-loading' : null,
        ],
        onClick: (event) => this.$emit('click', event),
      },
      [
        this.iconLeft ? h(BIcon, { icon: this.iconLeft, size: 'is-small' }) : null,
        ...renderDefaultSlot(this.$slots),
      ],
    );
  },
});

const BField = defineComponent({
  name: 'BField',
  props: {
    label: { type: String, default: '' },
    message: { type: String, default: '' },
    addons: { type: Boolean, default: false },
    grouped: { type: Boolean, default: false },
    expanded: { type: Boolean, default: false },
  },
  render() {
    return h('div', { class: ['field', this.grouped ? 'is-grouped' : null] }, [
      this.label ? h('label', { class: 'label' }, this.label) : null,
      h(
        'div',
        {
          class: [
            'control',
            this.addons ? 'has-addons' : null,
            this.expanded ? 'is-expanded' : null,
          ],
        },
        renderDefaultSlot(this.$slots),
      ),
      this.message
        ? h(
          'p',
          { class: 'help' },
          Array.isArray(this.message) ? this.message.filter(Boolean).join(' ') : this.message,
        )
        : null,
    ]);
  },
});

const BInput = defineComponent({
  name: 'BInput',
  inheritAttrs: false,
  props: {
    modelValue: { type: [String, Number], default: '' },
    modelModifiers: { type: Object, default: () => ({}) },
    type: { type: String, default: 'text' },
    placeholder: { type: String, default: '' },
    disabled: { type: Boolean, default: false },
    expanded: { type: Boolean, default: false },
    maxlength: { type: [String, Number], default: null },
    minlength: { type: [String, Number], default: null },
    icon: { type: String, default: '' },
  },
  emits: ['update:modelValue', 'input', 'keydown', 'keyup', 'change', 'focus', 'blur'],
  setup(
    props,
    {
      emit, attrs, expose, slots,
    },
  ) {
    const inputRef = ref(null);

    const normalizeValue = (value) => {
      if (props.modelModifiers && props.modelModifiers.number) {
        if (value === '') {
          return '';
        }
        const parsed = Number(value);
        return Number.isNaN(parsed) ? value : parsed;
      }

      return value;
    };

    const onInput = (event) => {
      const value = normalizeValue(event.target.value);
      emit('update:modelValue', value);
      emit('input', value);
    };

    expose({
      focus() {
        if (inputRef.value && typeof inputRef.value.focus === 'function') {
          inputRef.value.focus();
        }
      },
    });

    return () => {
      const inputProps = {
        ...attrs,
        ref: inputRef,
        class: ['input', props.type === 'textarea' ? 'textarea' : null, attrs.class],
        value: props.modelValue ?? '',
        placeholder: props.placeholder,
        disabled: props.disabled,
        maxlength: props.maxlength,
        minlength: props.minlength,
        onInput,
        onKeydown: (event) => emit('keydown', event),
        onKeyup: (event) => emit('keyup', event),
        onChange: (event) => emit('change', event),
        onFocus: (event) => emit('focus', event),
        onBlur: (event) => emit('blur', event),
      };

      const inputNode = props.type === 'textarea'
        ? h('textarea', inputProps)
        : h('input', { ...inputProps, type: props.type || 'text' });

      return h(
        'div',
        { class: ['control', props.icon ? 'has-icons-left' : null, props.expanded ? 'is-expanded' : null] },
        [
          inputNode,
          props.icon ? h(BIcon, { icon: props.icon, class: 'is-left' }) : null,
          ...renderDefaultSlot(slots),
        ],
      );
    };
  },
});

const BSelect = defineComponent({
  name: 'BSelect',
  inheritAttrs: false,
  props: {
    modelValue: { type: [String, Number, Boolean], default: '' },
    expanded: { type: Boolean, default: false },
    disabled: { type: Boolean, default: false },
  },
  emits: ['update:modelValue', 'change'],
  render() {
    return h(
      'div',
      { class: ['select', this.expanded ? 'is-fullwidth' : null] },
      [
        h(
          'select',
          {
            ...this.$attrs,
            value: this.modelValue,
            disabled: this.disabled,
            onChange: (event) => {
              this.$emit('update:modelValue', event.target.value);
              this.$emit('change', event);
            },
          },
          renderDefaultSlot(this.$slots),
        ),
      ],
    );
  },
});

function createCheckControl(name, inputType) {
  return defineComponent({
    name,
    props: {
      modelValue: { type: [String, Number, Boolean], default: false },
      nativeValue: { type: [String, Number, Boolean], default: true },
      disabled: { type: Boolean, default: false },
    },
    emits: ['update:modelValue', 'input', 'change'],
    render() {
      const checked = inputType === 'radio'
        ? this.modelValue === this.nativeValue
        : Boolean(this.modelValue);

      return h(
        'label',
        { class: [inputType === 'radio' ? 'radio' : 'checkbox', name === 'BRadioButton' ? 'radio-button' : null] },
        [
          h('input', {
            ...this.$attrs,
            type: inputType,
            disabled: this.disabled,
            checked,
            value: this.nativeValue,
            onChange: (event) => {
              const value = inputType === 'radio'
                ? this.nativeValue
                : event.target.checked;
              this.$emit('update:modelValue', value);
              this.$emit('input', value);
              this.$emit('change', value);
            },
          }),
          h('span', renderDefaultSlot(this.$slots)),
        ],
      );
    },
  });
}

const BCheckbox = createCheckControl('BCheckbox', 'checkbox');
const BRadio = createCheckControl('BRadio', 'radio');
const BRadioButton = createCheckControl('BRadioButton', 'radio');
const BSwitch = createCheckControl('BSwitch', 'checkbox');

const BTooltip = defineComponent({
  name: 'BTooltip',
  props: {
    label: { type: String, default: '' },
  },
  render() {
    return h('span', { title: this.label, class: 'legacy-tooltip' }, renderDefaultSlot(this.$slots));
  },
});

const BLoading = defineComponent({
  name: 'BLoading',
  props: {
    active: { type: Boolean, default: false },
    isFullPage: { type: Boolean, default: true },
  },
  render() {
    if (!this.active) {
      return null;
    }

    return h('div', { class: ['legacy-loading', this.isFullPage ? 'is-full-page' : 'is-inline'] }, [
      h('div', { class: 'legacy-loading-spinner' }),
    ]);
  },
});

const BProgress = defineComponent({
  name: 'BProgress',
  props: {
    value: { type: Number, default: 0 },
    max: { type: Number, default: 100 },
    showValue: { type: Boolean, default: false },
    type: { type: String, default: '' },
  },
  render() {
    return h('div', { class: ['legacy-progress', ...normalizeClass(this.type)] }, [
      h('progress', { max: this.max, value: this.value }, `${this.value}`),
      this.showValue ? h('span', { class: 'legacy-progress-value' }, `${Math.round(this.value)}%`) : null,
    ]);
  },
});

const BPagination = defineComponent({
  name: 'BPagination',
  props: {
    total: { type: Number, default: 0 },
    current: { type: Number, default: 1 },
    perPage: { type: Number, default: 10 },
    simple: { type: Boolean, default: false },
    paginationSimple: { type: Boolean, default: false },
  },
  emits: ['update:current', 'change'],
  computed: {
    pageCount() {
      if (!this.total || !this.perPage) {
        return 1;
      }
      return Math.max(1, Math.ceil(this.total / this.perPage));
    },
  },
  methods: {
    setPage(page) {
      const next = Math.min(this.pageCount, Math.max(1, page));
      this.$emit('update:current', next);
      this.$emit('change', next);
    },
  },
  render() {
    const pages = [];
    for (let page = 1; page <= this.pageCount; page += 1) {
      pages.push(
        h(
          'button',
          {
            type: 'button',
            class: ['pagination-link', page === this.current ? 'is-current' : null],
            onClick: () => this.setPage(page),
          },
          `${page}`,
        ),
      );
    }

    return h('nav', { class: 'pagination' }, [
      h('button', {
        type: 'button',
        class: 'pagination-previous',
        disabled: this.current <= 1,
        onClick: () => this.setPage(this.current - 1),
      }, 'Previous'),
      h('button', {
        type: 'button',
        class: 'pagination-next',
        disabled: this.current >= this.pageCount,
        onClick: () => this.setPage(this.current + 1),
      }, 'Next'),
      h('div', { class: 'pagination-list' }, pages),
    ]);
  },
});

const BCollapse = defineComponent({
  name: 'BCollapse',
  props: {
    modelValue: { type: Boolean, default: false },
    open: { type: Boolean, default: undefined },
  },
  emits: ['update:modelValue'],
  computed: {
    isOpen() {
      return typeof this.open === 'boolean' ? this.open : this.modelValue;
    },
  },
  render() {
    const trigger = this.$slots.trigger ? this.$slots.trigger({ open: this.isOpen }) : null;
    return h('div', { class: 'legacy-collapse' }, [
      trigger
        ? h('div', {
          class: 'legacy-collapse-trigger',
          onClick: () => this.$emit('update:modelValue', !this.isOpen),
        }, trigger)
        : null,
      this.isOpen ? h('div', { class: 'legacy-collapse-content' }, renderDefaultSlot(this.$slots)) : null,
    ]);
  },
});

const BUpload = defineComponent({
  name: 'BUpload',
  props: {
    modelValue: { type: [Array, Object, File], default: null },
    multiple: { type: Boolean, default: false },
    disabled: { type: Boolean, default: false },
  },
  emits: ['update:modelValue', 'input'],
  render() {
    return h('label', { class: ['legacy-upload', this.disabled ? 'is-disabled' : null] }, [
      h('input', {
        ...this.$attrs,
        type: 'file',
        multiple: this.multiple,
        disabled: this.disabled,
        onChange: (event) => {
          const files = Array.from(event.target.files || []);
          const value = this.multiple ? files : files[0] || null;
          this.$emit('update:modelValue', value);
          this.$emit('input', value);
        },
      }),
      h('div', { class: 'legacy-upload-content' }, renderDefaultSlot(this.$slots)),
    ]);
  },
});

const BDatepicker = defineComponent({
  name: 'BDatepicker',
  props: {
    modelValue: { type: [String, Date], default: null },
    disabled: { type: Boolean, default: false },
  },
  emits: ['update:modelValue'],
  methods: {
    formatValue(value) {
      if (!value) {
        return '';
      }
      const date = value instanceof Date ? value : new Date(value);
      if (Number.isNaN(date.getTime())) {
        return '';
      }
      return date.toISOString().slice(0, 10);
    },
  },
  render() {
    return h('input', {
      ...this.$attrs,
      class: ['input', this.$attrs.class],
      type: 'date',
      disabled: this.disabled,
      value: this.formatValue(this.modelValue),
      onInput: (event) => {
        const value = event.target.value ? new Date(`${event.target.value}T00:00:00`) : null;
        this.$emit('update:modelValue', value);
      },
    });
  },
});

const BAutocomplete = defineComponent({
  name: 'BAutocomplete',
  props: {
    modelValue: { type: String, default: '' },
    data: { type: Array, default: () => [] },
    field: { type: String, default: 'value' },
    placeholder: { type: String, default: '' },
    disabled: { type: Boolean, default: false },
    clearOnSelect: { type: Boolean, default: false },
    openOnFocus: { type: Boolean, default: false },
  },
  emits: ['update:modelValue', 'select'],
  setup(props, { emit }) {
    const isOpen = ref(false);

    const filteredItems = computed(() => props.data || []);

    const selectItem = (item) => {
      emit('select', item);
      emit('update:modelValue', props.clearOnSelect ? '' : (item && item[props.field]) || '');
      isOpen.value = false;
    };

    return () => h('div', { class: 'legacy-autocomplete' }, [
      h('input', {
        class: 'input',
        value: props.modelValue,
        placeholder: props.placeholder,
        disabled: props.disabled,
        onFocus: () => {
          if (props.openOnFocus) {
            isOpen.value = true;
          }
        },
        onInput: (event) => {
          emit('update:modelValue', event.target.value);
          isOpen.value = true;
        },
      }),
      isOpen.value && filteredItems.value.length
        ? h('div', { class: 'legacy-autocomplete-menu' }, filteredItems.value.map((item, index) => h(
          'button',
          {
            key: index,
            type: 'button',
            class: 'legacy-autocomplete-item',
            onClick: () => selectItem(item),
          },
          `${item[props.field] ?? item}`,
        )))
        : null,
    ]);
  },
});

const BTaginput = defineComponent({
  name: 'BTaginput',
  props: {
    modelValue: { type: Array, default: () => [] },
    data: { type: Array, default: () => [] },
    field: { type: String, default: 'value' },
    placeholder: { type: String, default: '' },
  },
  emits: ['update:modelValue', 'add', 'remove', 'typing'],
  setup(props, { emit }) {
    const query = ref('');

    const tagLabel = (item) => {
      if (item && typeof item === 'object') {
        return item[props.field] ?? item.value ?? '';
      }
      return item;
    };

    const addTag = (item) => {
      if (!item) {
        return;
      }
      emit('update:modelValue', [...props.modelValue, item]);
      emit('add', item);
      query.value = '';
    };

    return () => h('div', { class: 'legacy-taginput' }, [
      h('div', { class: 'tags' }, props.modelValue.map((item, index) => h(BTag, {
        key: `${tagLabel(item)}-${index}`,
        closable: true,
        onClose: () => {
          const next = props.modelValue.filter((_, currentIndex) => currentIndex !== index);
          emit('update:modelValue', next);
          emit('remove', item);
        },
      }, { default: () => [tagLabel(item)] }))),
      h('input', {
        class: 'input',
        value: query.value,
        placeholder: props.placeholder,
        onInput: (event) => {
          query.value = event.target.value;
          emit('typing', query.value);
        },
        onKeydown: (event) => {
          if (event.key === 'Enter' && query.value.trim()) {
            event.preventDefault();
            addTag(query.value.trim());
          }
        },
      }),
      props.data.length && query.value
        ? h('div', { class: 'legacy-autocomplete-menu' }, props.data.map((item, index) => h(
          'button',
          {
            key: index,
            type: 'button',
            class: 'legacy-autocomplete-item',
            onClick: () => addTag(item),
          },
          `${tagLabel(item)}`,
        )))
        : null,
    ]);
  },
});

const BModal = defineComponent({
  name: 'BModal',
  props: {
    active: { type: Boolean, default: false },
    width: { type: [String, Number], default: 640 },
    canCancel: { type: [Boolean, Array], default: true },
  },
  emits: ['update:active', 'close'],
  methods: {
    close() {
      this.$emit('update:active', false);
      this.$emit('close');
    },
  },
  render() {
    if (!this.active) {
      return null;
    }

    return h(Teleport, { to: 'body' }, [
      h('div', { class: 'legacy-modal' }, [
        h('div', {
          class: 'legacy-modal-background',
          onClick: () => {
            if (this.canCancel !== false) {
              this.close();
            }
          },
        }),
        h(
          'div',
          {
            class: 'legacy-modal-content',
            style: {
              maxWidth: typeof this.width === 'number' ? `${this.width}px` : this.width,
              width: 'calc(100vw - 32px)',
            },
          },
          renderDefaultSlot(this.$slots),
        ),
      ]),
    ]);
  },
});

const BTableColumn = defineComponent({
  name: 'BTableColumn',
  legacyTableColumn: true,
  props: {
    label: { type: String, default: '' },
    field: { type: String, default: '' },
    headerClass: { type: String, default: '' },
    cellClass: { type: String, default: '' },
    tdAttrs: { type: [Function, Object], default: null },
  },
  render() {
    return null;
  },
});

function flattenNodes(nodes, out = []) {
  nodes.forEach((node) => {
    if (Array.isArray(node)) {
      flattenNodes(node, out);
      return;
    }
    if (node && Array.isArray(node.children)) {
      flattenNodes(node.children, out);
    }
    if (node) {
      out.push(node);
    }
  });
  return out;
}

const BTable = defineComponent({
  name: 'BTable',
  props: {
    data: { type: Array, default: () => [] },
    loading: { type: Boolean, default: false },
    checkable: { type: Boolean, default: false },
    checkedRows: { type: Array, default: () => [] },
  },
  emits: ['update:checkedRows', 'check', 'check-all'],
  methods: {
    isChecked(row) {
      return this.checkedRows.some((item) => item === row || item.id === row.id);
    },
    toggleRow(row, checked) {
      const next = checked
        ? [...this.checkedRows.filter((item) => item !== row && item.id !== row.id), row]
        : this.checkedRows.filter((item) => item !== row && item.id !== row.id);
      this.$emit('update:checkedRows', next);
      this.$emit('check', row);
    },
    toggleAll(checked) {
      const next = checked ? [...this.data] : [];
      this.$emit('update:checkedRows', next);
      this.$emit('check-all', next);
    },
  },
  render() {
    const nodes = flattenNodes(renderDefaultSlot(this.$slots));
    const columns = nodes.filter((node) => node.type && node.type.legacyTableColumn);

    const head = h('thead', [
      h('tr', [
        this.checkable
          ? h('th', [
            h('input', {
              type: 'checkbox',
              checked: this.data.length > 0 && this.data.every((row) => this.isChecked(row)),
              onChange: (event) => this.toggleAll(event.target.checked),
            }),
          ])
          : null,
        ...columns.map((column, index) => h(
          'th',
          {
            key: `${column.props?.field || column.props?.label || 'col'}-${index}`,
            class: column.props?.headerClass,
          },
          column.props?.label || '',
        )),
      ]),
    ]);

    const body = h('tbody', this.data.map((row, rowIndex) => h('tr', { key: row.id || rowIndex }, [
      this.checkable
        ? h('td', [
          h('input', {
            type: 'checkbox',
            checked: this.isChecked(row),
            onChange: (event) => this.toggleRow(row, event.target.checked),
          }),
        ])
        : null,
      ...columns.map((column, columnIndex) => {
        const tdAttrs = typeof column.props?.tdAttrs === 'function'
          ? column.props.tdAttrs(row)
          : (column.props?.tdAttrs || {});
        const slot = column.children && column.children.default
          ? column.children.default({ row, index: rowIndex })
          : [row[column.props?.field]];
        return h(
          'td',
          {
            ...tdAttrs,
            key: `${column.props?.field || 'cell'}-${columnIndex}`,
            class: column.props?.cellClass,
          },
          slot,
        );
      }),
    ])));

    return h('div', { class: 'legacy-table-wrap' }, [
      this.loading ? h(BLoading, { active: true, isFullPage: false }) : null,
      h('table', { class: 'table is-fullwidth is-hoverable' }, [head, body]),
    ]);
  },
});

const legacyComponents = {
  BAutocomplete,
  BButton,
  BCheckbox,
  BCollapse,
  BDatepicker,
  BField,
  BIcon,
  BInput,
  BLoading,
  BModal,
  BPagination,
  BProgress,
  BRadio,
  BRadioButton,
  BSelect,
  BSwitch,
  BTable,
  BTableColumn,
  BTag,
  BTaginput,
  BTaglist,
  BTooltip,
  BUpload,
};

export function registerLegacyUI(app) {
  Object.entries(legacyComponents).forEach(([name, component]) => {
    app.component(name, component);
  });
}

export function installLegacyUIStyles() {
  nextTick(() => {
    if (document.getElementById('legacy-ui-styles')) {
      return;
    }

    const style = document.createElement('style');
    style.id = 'legacy-ui-styles';
    style.textContent = `
      .legacy-loading { align-items:center; display:flex; inset:0; justify-content:center; position:absolute; }
      .legacy-loading.is-full-page { background:rgba(255,255,255,.7); position:fixed; z-index:2500; }
      .legacy-loading-spinner { animation:legacy-spin .8s linear infinite; border:3px solid #d0d5dd; border-top-color:#0f5bd8; border-radius:999px; height:36px; width:36px; }
      .tag-close-button { background:none; border:0; cursor:pointer; margin-left:6px; }
      .legacy-modal { align-items:center; display:flex; inset:0; justify-content:center; position:fixed; z-index:2400; }
      .legacy-modal-background { background:rgba(15,23,42,.45); inset:0; position:absolute; }
      .legacy-modal-content { position:relative; z-index:1; }
      .legacy-upload input { display:none; }
      .legacy-upload-content { cursor:pointer; }
      .legacy-programmatic-toast {
        background:#1d2939;
        border-radius:10px;
        bottom:24px;
        color:#fff;
        opacity:0;
        padding:12px 16px;
        position:fixed;
        right:24px;
        transform:translateY(8px);
        transition:opacity .2s ease, transform .2s ease;
        z-index:2600;
      }
      .legacy-programmatic-toast.is-visible { opacity:1; transform:translateY(0); }
      .legacy-programmatic-toast.is-danger { background:#b42318; }
      .legacy-autocomplete { position:relative; }
      .legacy-autocomplete-menu {
        background:#fff;
        border:1px solid #d0d5dd;
        border-radius:10px;
        box-shadow:0 12px 32px rgba(15,23,42,.12);
        left:0;
        margin-top:6px;
        max-height:240px;
        overflow:auto;
        position:absolute;
        right:0;
        top:100%;
        z-index:20;
      }
      .legacy-autocomplete-item { background:none; border:0; cursor:pointer; display:block; padding:10px 12px; text-align:left; width:100%; }
      .legacy-taginput { position:relative; }
      .legacy-taginput .tags { margin-bottom:8px; }
      .pagination { align-items:center; display:flex; gap:8px; flex-wrap:wrap; }
      .pagination-list { display:flex; gap:6px; flex-wrap:wrap; }
      .pagination-link.is-current { background:#0f5bd8; color:#fff; }
      .radio-button span { border:1px solid #d0d5dd; border-radius:8px; display:inline-block; padding:8px 12px; }
      @keyframes legacy-spin { to { transform:rotate(360deg); } }
    `;
    document.head.appendChild(style);
  });
}
