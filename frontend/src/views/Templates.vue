<template>
  <section class="templates-page">
    <header class="page-header">
      <div class="header-content">
        <h1 class="text-h4">
          {{ $t('globals.terms.templates') }}
          <span v-if="templates.length > 0">({{ templates.length }})</span>
        </h1>
      </div>
      <div class="header-actions">
        <v-btn
          v-if="$can('templates:manage')"
          color="primary"
          prepend-icon="mdi-plus"
          data-cy="btn-new"
          @click="showNewForm"
        >
          {{ $t('globals.buttons.new') }}
        </v-btn>
      </div>
    </header>

    <div class="table-wrap">
      <v-data-table
        :headers="tableHeaders"
        :items="templates"
        :loading="loading.templates"
        :sort-by="defaultSortBy"
        class="admin-data-table templates-table"
        item-value="id"
      >
        <template #[`item.name`]="{ item }">
          <div>
            <button
              type="button"
              class="link-button"
              data-cy="template-name"
              @click.stop.prevent="showEditForm(item)"
            >
              {{ item.name }}
            </button>
            <div class="template-meta mt-2">
              <v-chip v-if="item.isDefault" size="small" variant="tonal">
                {{ $t('templates.default') }}
              </v-chip>
              <div v-if="item.type === 'tx'" class="text-caption text-medium-emphasis">
                {{ item.subject }}
              </div>
            </div>
          </div>
        </template>

        <template #[`item.type`]="{ item }">
          <v-chip size="small" :class="['template-type-chip', item.type]" :data-cy="`type-${item.type}`">
            {{ templateTypeLabel(item.type) }}
          </v-chip>
        </template>

        <template #[`item.createdAt`]="{ item }">
          {{ $utils.niceDate(item.createdAt) }}
        </template>

        <template #[`item.updatedAt`]="{ item }">
          {{ $utils.niceDate(item.updatedAt) }}
        </template>

        <template #[`item.actions`]="{ item }">
          <div class="action-group">
            <v-tooltip :text="$t('templates.preview')" location="top">
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  icon="mdi-file-find-outline"
                  size="x-small"
                  variant="text"
                  data-cy="btn-preview"
                  @click="previewTemplate(item)"
                />
              </template>
            </v-tooltip>

            <v-tooltip :text="$t('globals.buttons.edit')" location="top">
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  icon="mdi-pencil-outline"
                  size="x-small"
                  variant="text"
                  data-cy="btn-edit"
                  @click="showEditForm(item)"
                />
              </template>
            </v-tooltip>

            <v-tooltip :text="$t('globals.buttons.clone')" location="top">
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  icon="mdi-file-multiple-outline"
                  size="x-small"
                  variant="text"
                  data-cy="btn-clone"
                  @click="$utils.prompt('Clone template', { placeholder: 'Name', value: `Copy of ${item.name}` }, (name) => cloneTemplate(name, item))"
                />
              </template>
            </v-tooltip>

            <v-tooltip
              v-if="!item.isDefault && item.type === 'campaign'"
              :text="$t('templates.makeDefault')"
              location="top"
            >
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  icon="mdi-check-circle-outline"
                  size="x-small"
                  variant="text"
                  data-cy="btn-set-default"
                  @click="$utils.confirm(null, () => makeTemplateDefault(item))"
                />
              </template>
            </v-tooltip>
            <v-btn
              v-else
              icon="mdi-check-circle-outline"
              size="x-small"
              variant="text"
              disabled
            />

            <v-tooltip
              v-if="!item.isDefault"
              :text="$t('globals.buttons.delete')"
              location="top"
            >
              <template #activator="{ props }">
                <v-btn
                  v-bind="props"
                  icon="mdi-trash-can-outline"
                  size="x-small"
                  variant="text"
                  color="error"
                  data-cy="btn-delete"
                  @click="$utils.confirm(null, () => deleteTemplate(item))"
                />
              </template>
            </v-tooltip>
            <v-btn
              v-else
              icon="mdi-trash-can-outline"
              size="x-small"
              variant="text"
              disabled
            />
          </div>
        </template>

        <template #no-data>
          <empty-placeholder v-if="!loading.templates" />
        </template>
      </v-data-table>
    </div>

    <v-overlay
      :model-value="isFormVisible"
      class="admin-overlay align-center justify-center"
      scrim="rgba(15, 23, 42, 0.45)"
      @update:model-value="handleDialogModelUpdate"
    >
      <div class="admin-dialog-frame template-dialog-frame">
        <template-form
          v-if="isFormVisible"
          :data="formData"
          :is-editing="isEditing"
          @finished="formFinished"
          @close="closeForm"
        />
      </div>
    </v-overlay>

    <campaign-preview
      v-if="previewItem"
      type="template"
      :id="previewItem.id"
      :template-type="previewItem.type"
      :title="previewItem.name"
      @close="closePreview"
    />
  </section>
</template>

<script>
import { mapState } from 'vuex';
import CampaignPreview from '../components/CampaignPreview.vue';
import EmptyPlaceholder from '../components/EmptyPlaceholder.vue';
import TemplateForm from './TemplateForm.vue';

export default {
  name: 'TemplatesPage',

  components: {
    CampaignPreview,
    TemplateForm,
    EmptyPlaceholder,
  },

  data() {
    return {
      curItem: null,
      isEditing: false,
      isFormVisible: false,
      previewItem: null,
      defaultSortBy: [{ key: 'createdAt', order: 'asc' }],
    };
  },

  computed: {
    ...mapState(['templates', 'loading']),

    formData() {
      if (this.isEditing) {
        return this.curItem || {};
      }

      return {
        type: 'campaign',
        name: '',
        subject: '',
        body: '',
        bodySource: '',
        ...(this.curItem || {}),
      };
    },

    tableHeaders() {
      return [
        { title: this.$t('globals.fields.name'), key: 'name' },
        { title: this.$t('globals.fields.type'), key: 'type' },
        { title: this.$t('globals.fields.id'), key: 'id' },
        { title: this.$t('globals.fields.createdAt'), key: 'createdAt' },
        { title: this.$t('globals.fields.updatedAt'), key: 'updatedAt' },
        {
          title: '',
          key: 'actions',
          align: 'end',
          sortable: false,
          width: 180,
        },
      ];
    },
  },

  methods: {
    fetchTemplates() {
      return this.$api.getTemplates();
    },

    templateTypeLabel(type) {
      if (type === 'campaign') {
        return this.$tc('templates.typeCampaignHTML');
      }

      if (type === 'campaign_visual') {
        return this.$tc('templates.typeCampaignVisual');
      }
      if (type === 'campaign_grapes_mjml') {
        return 'GrapesJS (MJML)';
      }

      return this.$tc('templates.typeTransactional');
    },

    showEditForm(data) {
      this.curItem = data;
      this.isFormVisible = true;
      this.isEditing = true;
    },

    showNewForm(type = 'campaign') {
      const nextType = typeof type === 'string' && type ? type : 'campaign';
      this.curItem = { type: nextType };
      this.isFormVisible = true;
      this.isEditing = false;
    },

    applyRouteAction(templatesData = this.templates) {
      const { open, type, edit } = this.$route.query;
      if (typeof edit === 'string' && edit) {
        const match = (templatesData || []).find((item) => item.id === edit);
        if (match) {
          this.showEditForm(match);
        }
        return;
      }

      if (open === 'new') {
        this.showNewForm(typeof type === 'string' && type ? type : 'campaign');
      }
    },

    formFinished() {
      this.fetchTemplates();
    },

    closeForm() {
      this.isFormVisible = false;
      this.curItem = null;
      this.isEditing = false;
    },

    handleDialogModelUpdate(value) {
      if (!value) {
        this.closeForm();
      }
    },

    previewTemplate(template) {
      this.previewItem = template;
    },

    closePreview() {
      this.previewItem = null;
    },

    cloneTemplate(name, template) {
      const data = {
        name,
        type: template.type,
        subject: template.subject,
        body: template.body,
        body_source: template.bodySource,
      };

      this.$api.createTemplate(data).then((resp) => {
        this.fetchTemplates();
        this.$utils.toast(`'${resp.name}' created`);
      });
    },

    makeTemplateDefault(template) {
      this.$api.makeTemplateDefault(template.id).then(() => {
        this.fetchTemplates();
        this.$utils.toast(this.$t('globals.messages.created', { name: template.name }));
      });
    },

    deleteTemplate(template) {
      this.$api.deleteTemplate(template.id).then(() => {
        this.fetchTemplates();
        this.$utils.toast(this.$t('globals.messages.deleted', { name: template.name }));
      });
    },
  },

  created() {
    this.$events.$on('page.refresh', this.fetchTemplates);
  },

  beforeUnmount() {
    this.$events.$off('page.refresh', this.fetchTemplates);
  },

  mounted() {
    this.fetchTemplates().then((data) => {
      this.applyRouteAction(data);
    });
  },
};
</script>

<style scoped>
.page-header {
  align-items: center;
  display: flex;
  gap: 16px;
  justify-content: space-between;
  margin-bottom: 16px;
}

.header-content h1 {
  line-height: 1.25;
  margin: 0;
}

.header-actions {
  align-items: center;
  display: flex;
  justify-content: flex-end;
}

.table-wrap {
  overflow-x: auto;
}

.action-group {
  display: flex;
  justify-content: flex-end;
  gap: 0.25rem;
}

.link-button {
  background: none;
  border: 0;
  color: rgb(var(--v-theme-primary));
  cursor: pointer;
  font: inherit;
  padding: 0;
  text-align: left;
}

.link-button:hover {
  text-decoration: underline;
}

.template-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
}

.template-type-chip.campaign {
  background: rgba(var(--v-theme-primary), 0.12);
}

.template-type-chip.campaign_visual {
  background: rgba(var(--v-theme-success), 0.12);
}

.template-type-chip.campaign_grapes_mjml {
  background: rgba(var(--v-theme-info), 0.14);
}

.template-type-chip.tx {
  background: rgba(var(--v-theme-warning), 0.16);
}

.admin-overlay {
  padding: 16px;
  z-index: 2400;
}

.admin-dialog-frame {
  background: transparent;
  box-shadow: none;
  max-height: calc(100vh - 32px);
  overflow: hidden;
  width: auto;
}

.template-dialog-frame {
  display: flex;
  max-height: calc(100vh - 32px);
}

@media (max-width: 960px) {
  .page-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .header-actions {
    justify-content: flex-start;
  }
}
</style>
