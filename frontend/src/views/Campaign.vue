<template>
  <section class="campaign">
    <header class="campaign-header mb-6">
      <v-row class="align-center" dense>
        <v-col cols="12" md="6">
          <div v-if="isEditing && data.status" class="d-flex flex-wrap align-center ga-2 mb-3">
            <v-chip :class="data.status" size="small">
              {{ $t(`campaigns.status.${data.status}`) }}
            </v-chip>
            <v-chip v-if="data.type === 'optin'" :class="data.type" size="small">
              {{ $t('lists.optin') }}
            </v-chip>
            <span
              v-if="isEditing"
              class="inline-meta"
              :data-campaign-id="data.id"
            >
              {{ $t('globals.fields.id') }}: <copy-text :text="`${data.id}`" />
              {{ $t('globals.fields.uuid') }}: <copy-text :text="data.uuid" />
            </span>
          </div>
          <h1 class="text-h4 font-weight-medium mb-0">
            {{ isEditing ? data.name : $t('campaigns.newCampaign') }}
          </h1>
        </v-col>

        <v-col cols="12" md="6">
          <div
            v-if="canManage && isEditing && canEdit"
            class="campaign-actions d-flex flex-wrap justify-md-end ga-2"
          >
            <v-btn
              type="button"
              color="primary"
              class="action-btn"
              :disabled="loading.campaigns"
              :loading="loading.campaigns"
              data-cy="btn-save"
              aria-keyshortcuts="ctrl+s"
              @click="onSubmit('update')"
            >
              <v-icon start icon="mdi-content-save-outline" />
              <span class="has-kbd">{{ $t('globals.buttons.saveChanges') }} <span class="kbd">Ctrl+S</span></span>
            </v-btn>
            <v-btn
              v-if="canStart"
              type="button"
              color="primary"
              class="action-btn"
              :disabled="loading.campaigns"
              :loading="loading.campaigns"
              data-cy="btn-start"
              @click="startCampaign"
            >
              <v-icon start icon="mdi-rocket-launch-outline" />
              <span>{{ $t('campaigns.start') }}</span>
            </v-btn>
            <v-btn
              v-if="canSchedule"
              type="button"
              color="primary"
              class="action-btn"
              :disabled="loading.campaigns"
              :loading="loading.campaigns"
              data-cy="btn-schedule"
              @click="startCampaign"
            >
              <v-icon start icon="mdi-clock-start" />
              <span>{{ $t('campaigns.schedule') }}</span>
            </v-btn>
            <v-btn
              v-if="canUnSchedule"
              type="button"
              color="primary"
              class="action-btn"
              :disabled="loading.campaigns"
              :loading="loading.campaigns"
              data-cy="btn-unschedule"
              @click="$utils.confirm(null, unscheduleCampaign)"
            >
              <v-icon start icon="mdi-clock-start" />
              <span>{{ $t('campaigns.unSchedule') }}</span>
            </v-btn>
          </div>
        </v-col>
      </v-row>
    </header>

    <v-progress-linear
      v-if="loading.campaigns"
      color="primary"
      indeterminate
      class="mb-4"
    />

    <v-tabs
      v-model="activeTab"
      class="campaign-tabs mb-6"
      color="primary"
      align-tabs="start"
      show-arrows
      @update:model-value="onTab"
    >
      <v-tab value="campaign" prepend-icon="mdi-rocket-launch-outline">
        {{ $tc('globals.terms.campaign') }}
      </v-tab>
      <v-tab value="content" prepend-icon="mdi-text" :disabled="isNew">
        {{ $t('campaigns.content') }}
      </v-tab>
      <v-tab value="attribs" prepend-icon="mdi-code-tags" :disabled="isNew">
        {{ $t('globals.terms.attribs') }}
      </v-tab>
      <v-tab value="archive" prepend-icon="mdi-newspaper-variant-outline" :disabled="isNew">
        {{ $t('campaigns.archive') }}
      </v-tab>
    </v-tabs>

    <section v-show="activeTab === 'campaign'">
      <v-row>
        <v-col cols="12" lg="8">
          <v-form @submit.prevent="() => onSubmit(isNew ? 'create' : 'update')">
            <v-text-field
              ref="focus"
              v-model="form.name"
              :label="$t('globals.fields.name')"
              maxlength="200"
              name="name"
              :disabled="!canEdit"
              :placeholder="$t('globals.fields.name')"
              required
              type="text"
              variant="outlined"
              density="comfortable"
              class="mb-4"
            />

            <v-text-field
              v-model="form.subject"
              :label="$t('campaigns.subject')"
              maxlength="5000"
              name="subject"
              :disabled="!canEdit"
              :placeholder="$t('campaigns.subject')"
              required
              type="text"
              variant="outlined"
              density="comfortable"
              class="mb-4"
            />

            <v-text-field
              v-model="form.fromEmail"
              :label="$t('campaigns.fromAddress')"
              maxlength="200"
              name="from_email"
              :disabled="!canEdit"
              :placeholder="$t('campaigns.fromAddressPlaceholder')"
              required
              type="text"
              variant="outlined"
              density="comfortable"
              class="mb-4"
            />

            <v-select
              :model-value="selectedListIds"
              :items="availableLists"
              :label="$t('globals.terms.lists')"
              :disabled="!canEdit"
              multiple
              chips
              closable-chips
              item-title="name"
              item-value="listValue"
              variant="outlined"
              class="mb-4"
              @update:model-value="onListsChange"
            />

            <v-row>
              <v-col cols="12" md="6">
                <v-select
                  v-model="form.messenger"
                  :items="availableMessengers"
                  :label="$tc('globals.terms.messenger')"
                  name="messenger"
                  :disabled="!canEdit"
                  required
                  variant="outlined"
                  class="mb-4"
                />
              </v-col>
              <v-col cols="12" md="6">
                <v-select
                  v-model="form.content.contentType"
                  :items="contentTypeOptions"
                  :label="$t('campaigns.format')"
                  item-title="title"
                  item-value="value"
                  :disabled="!canEdit || isEditing"
                  name="format"
                  variant="outlined"
                  class="mb-4"
                />
              </v-col>
            </v-row>

            <v-text-field
              v-model="tagsInput"
              :aria-label="$t('globals.terms.tags')"
              :label="$t('globals.terms.tags')"
              :disabled="!canEdit"
              :placeholder="$t('globals.terms.tags')"
              type="text"
              variant="outlined"
              density="comfortable"
              class="mb-4"
            />

            <v-divider class="my-6" />

            <v-row class="align-center">
              <v-col cols="12" md="4">
                <div data-cy="btn-send-later">
                  <v-checkbox
                    v-model="form.sendLater"
                    :disabled="!canEdit"
                    :label="$t('campaigns.sendLater')"
                    hide-details
                    density="comfortable"
                  />
                </div>
              </v-col>
              <v-col cols="12" md="8">
                <div v-if="form.sendLater" data-cy="send_at">
                  <v-text-field
                    :model-value="toDateTimeLocal(form.sendAtDate)"
                    :disabled="!canEdit"
                    required
                    type="datetime-local"
                    :label="$t('campaigns.dateAndTime')"
                    variant="outlined"
                    density="comfortable"
                    @update:model-value="onSendAtInput"
                  />
                  <p v-if="form.sendAtDate" class="form-help">
                    {{ $utils.duration(Date(), form.sendAtDate) }}
                  </p>
                </div>
              </v-col>
            </v-row>

            <div class="d-flex justify-end">
              <v-btn
                variant="text"
                prepend-icon="mdi-plus"
                data-cy="btn-headers"
                @click="onShowHeaders"
              >
                {{ $t('settings.smtp.setCustomHeaders') }}
              </v-btn>
            </div>

            <div v-if="form.headersStr !== '[]' || isHeadersVisible">
              <v-textarea
                v-model="form.headersStr"
                name="headers"
                variant="outlined"
                auto-grow
                rows="4"
                placeholder="[{&quot;X-Custom&quot;: &quot;value&quot;}, {&quot;X-Custom2&quot;: &quot;value&quot;}]"
                :disabled="!canEdit"
              />
              <p class="form-help">{{ $t('campaigns.customHeadersHelp') }}</p>
            </div>

            <v-divider class="my-6" />

            <div v-if="isNew">
              <v-btn
                type="submit"
                color="primary"
                :disabled="loading.campaigns"
                :loading="loading.campaigns"
                data-cy="btn-continue"
              >
                {{ $t('campaigns.continue') }}
              </v-btn>
            </div>
          </v-form>
        </v-col>

        <v-col v-if="canManage" cols="12" lg="4">
          <v-card variant="outlined">
            <v-card-title class="text-subtitle-1">
              {{ $t('campaigns.sendTest') }}
            </v-card-title>
            <v-card-text>
              <v-textarea
                v-model="testEmailsInput"
                :aria-label="$t('campaigns.testEmails')"
                :disabled="isNew"
                :placeholder="$t('campaigns.testEmails')"
                rows="3"
                auto-grow
                variant="outlined"
              />
              <p class="form-help">{{ $t('campaigns.sendTestHelp') }}</p>
              <v-btn
                type="button"
                color="primary"
                :disabled="loading.campaigns || isNew"
                :loading="loading.campaigns"
                class="mt-4"
                @click="onSubmit('test')"
              >
                <v-icon start icon="mdi-email-outline" />
                <span>{{ $t('campaigns.send') }}</span>
              </v-btn>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>
    </section>

    <section v-show="activeTab === 'content'">
      <editor
        v-if="data.id"
        :key="editorKey"
        v-model="form.content"
        :id="data.id"
        :title="data.name"
        :disabled="!canEdit"
        :templates="templates"
        :content-types="contentTypes"
      />

      <v-row class="mt-4">
        <v-col cols="12" md="6">
          <v-btn
            v-if="!isAttachFieldVisible"
            variant="text"
            prepend-icon="mdi-file-upload-outline"
            data-cy="btn-attach"
            @click="onShowAttachField"
          >
            {{ $t('campaigns.addAttachments') }}
          </v-btn>

          <div v-if="isAttachFieldVisible" data-cy="media">
            <v-card variant="outlined" class="attachment-card">
              <v-card-text>
                <div class="text-subtitle-2 mb-2">{{ $t('campaigns.attachments') }}</div>
                <div class="d-flex flex-wrap ga-2 mb-3">
                  <v-chip
                    v-for="(item, index) in form.media"
                    :key="item.id || index"
                    closable
                    size="small"
                    @click:close="form.media.splice(index, 1)"
                  >
                    {{ item.filename }}
                  </v-chip>
                </div>
                <v-text-field
                  ref="media"
                  :label="$t('campaigns.attachments')"
                  :disabled="!canEdit"
                  readonly
                  variant="outlined"
                  @focus="onOpenAttach"
                />
              </v-card-text>
            </v-card>
          </div>
        </v-col>

        <v-col cols="12" md="6" class="text-md-right">
          <a
            href="https://listmonk.app/docs/templating/#template-expressions"
            target="_blank"
            rel="noopener noreferer"
            class="templating-link"
          >
            <v-icon size="small">mdi-code-tags</v-icon> {{ $t('campaigns.templatingRef') }}
          </a>

          <div v-if="canEdit && form.content.contentType !== 'plain'" class="mt-3">
            <v-btn
              v-if="form.altbody === null"
              variant="text"
              size="small"
              prepend-icon="mdi-text"
              @click="onAddAltBody"
            >
              {{ $t('campaigns.addAltText') }}
            </v-btn>
            <v-btn
              v-else
              variant="text"
              size="small"
              prepend-icon="mdi-trash-can-outline"
              @click="$utils.confirm(null, onRemoveAltBody)"
            >
              {{ $t('campaigns.removeAltText') }}
            </v-btn>
          </div>
        </v-col>
      </v-row>

      <div v-if="canEdit && form.content.contentType !== 'plain' && form.altbody !== null" class="mt-4">
        <v-textarea
          v-model="form.altbody"
          :disabled="!canEdit"
          variant="outlined"
          auto-grow
          rows="6"
        />
      </div>
    </section>

    <section v-show="activeTab === 'attribs'">
      <v-textarea
        v-model="form.attribsStr"
        :label="$t('globals.terms.attribs')"
        variant="outlined"
        :disabled="!canEdit"
        rows="15"
        auto-grow
      />
      <p class="form-help">{{ $t('campaigns.attribsHelp') }}</p>
    </section>

    <section v-show="activeTab === 'archive'">
      <v-row>
        <v-col cols="12" md="4">
          <div data-cy="btn-archive">
            <v-switch
              v-model="form.archive"
              :disabled="!canArchive"
              :label="$t('campaigns.archiveEnable')"
              data-cy="btn-archive"
              hide-details
            />

            <v-btn
              :href="`${serverConfig.root_url}/archive/${data.uuid}`"
              target="_blank"
              rel="noopener noreferer"
              variant="text"
              icon
              :disabled="!form.archive"
              :aria-label="$t('campaigns.archive')"
              class="mb-2"
            >
              <v-icon>mdi-link-variant</v-icon>
            </v-btn>

            <p class="form-help">{{ $t('campaigns.archiveHelp') }}</p>
          </div>
        </v-col>

        <v-col cols="12" md="8" class="text-md-right">
          <v-btn
            v-if="!canEdit && canArchive"
            type="button"
            color="primary"
            :disabled="loading.campaigns"
            :loading="loading.campaigns"
            data-cy="btn-save"
            @click="onUpdateCampaignArchive"
          >
            <v-icon start icon="mdi-content-save-outline" />
            <span>{{ $t('globals.buttons.saveChanges') }}</span>
          </v-btn>
        </v-col>
      </v-row>

      <v-row class="mt-1">
        <v-col cols="12" md="6">
          <v-select
            v-model="form.archiveTemplateId"
            :items="archiveTemplates"
            item-title="name"
            item-value="id"
            :label="$tc('globals.terms.template')"
            name="template"
            :disabled="!canArchive || !form.archive || form.content.contentType === 'visual'"
            required
            variant="outlined"
          />
        </v-col>

        <v-col cols="12" md="6" class="text-md-right">
          <v-btn
            v-if="form.archive && (!form.archiveMetaStr || form.archiveMetaStr === '{}')"
            color="primary"
            variant="outlined"
            icon="mdi-code-tags"
            :aria-label="$t('campaigns.archiveMeta')"
            class="mr-2"
            @click="onFillArchiveMeta"
          />
          <v-btn
            v-if="form.archive"
            type="button"
            color="primary"
            data-cy="btn-preview"
            prepend-icon="mdi-file-find-outline"
            @click="onToggleArchivePreview"
          >
            <span>{{ $t('campaigns.preview') }}</span>
          </v-btn>
        </v-col>
      </v-row>

      <v-text-field
        ref="archiveSlug"
        v-model="form.archiveSlug"
        :label="$t('campaigns.archiveSlug')"
        maxlength="200"
        name="archive_slug"
        data-cy="archive-slug"
        :disabled="!canArchive || !form.archive"
        type="text"
        variant="outlined"
        class="mt-2"
      />
      <p class="form-help">{{ $t('campaigns.archiveSlugHelp') }}</p>

      <v-textarea
        v-model="form.archiveMetaStr"
        :label="$t('campaigns.archiveMeta')"
        name="archive_meta"
        data-cy="archive-meta"
        :disabled="!canArchive || !form.archive"
        rows="20"
        auto-grow
        variant="outlined"
      />
      <p class="form-help">{{ $t('campaigns.archiveMetaHelp') }}</p>
    </section>

    <v-dialog
      v-model="isAttachModalOpen"
      :aria-modal="true"
      max-width="900"
      scrollable
    >
      <v-card>
        <v-card-text>
          <media is-modal type="legacy-attachment" @selected="onAttachSelect" />
        </v-card-text>
      </v-card>
    </v-dialog>

    <campaign-preview
      v-if="isPreviewingArchive"
      @close="onToggleArchivePreview"
      type="campaign"
      :id="data.id"
      :archive-meta="form.archiveMetaStr"
      :title="data.title"
      :content-type="data.contentType"
      :template-id="form.archiveTemplateId"
      is-post
      is-archive
    />
  </section>
</template>

<script>
import dayjs from 'dayjs';
import htmlToPlainText from 'textversionjs';
import { mapState } from 'vuex';

import CampaignPreview from '../components/CampaignPreview.vue';
import CopyText from '../components/CopyText.vue';
import Editor from '../components/Editor.vue';
import Media from './Media.vue';

export default {
  components: {
    Editor,
    Media,
    CopyText,
    CampaignPreview,
  },

  data() {
    return {
      contentTypes: Object.freeze({
        richtext: this.$t('campaigns.richText'),
        html: this.$t('campaigns.rawHTML'),
        markdown: this.$t('campaigns.markdown'),
        plain: this.$t('campaigns.plainText'),
        visual: this.$t('campaigns.visual'),
      }),

      isNew: false,
      isEditing: false,
      isHeadersVisible: false,
      isAttachFieldVisible: false,
      isAttachModalOpen: false,
      isPreviewingArchive: false,
      activeTab: 'campaign',

      data: {},

      // IDs from ?list_id query param.
      selListIDs: [],

      // Binds form input values.
      form: {
        archiveSlug: '',
        name: '',
        subject: '',
        fromEmail: '',
        headersStr: '[]',
        headers: [],
        attribsStr: '{}',
        messenger: 'email',
        lists: [],
        tags: [],
        sendAt: null,
        content: {
          contentType: 'richtext',
          body: '',
          bodySource: null,
          templateId: null,
        },
        altbody: null,
        media: [],

        // Parsed Date() version of send_at from the API.
        sendAtDate: null,
        sendLater: false,
        archive: false,
        archiveMetaStr: '{}',
        archiveMeta: {},
        testEmails: [],
      },
    };
  },

  methods: {
    formatDateTime(s) {
      return dayjs(s).format('YYYY-MM-DD HH:mm');
    },

    toDateTimeLocal(value) {
      return value ? dayjs(value).format('YYYY-MM-DDTHH:mm') : '';
    },

    onSendAtInput(value) {
      this.form.sendAtDate = value ? dayjs(value).toDate() : null;
    },

    onListsChange(selectedIDs = []) {
      const normalizedIDs = Array.isArray(selectedIDs)
        ? selectedIDs.map((id) => String(id))
        : [];
      const listMap = new Map(this.availableLists.map((list) => [list.listValue, list]));
      this.form.lists = normalizedIDs
        .map((id) => listMap.get(id))
        .filter(Boolean);
    },

    onToggleArchivePreview() {
      this.isPreviewingArchive = !this.isPreviewingArchive;
    },

    onAddAltBody() {
      this.form.altbody = htmlToPlainText(this.form.content.body);
    },

    onRemoveAltBody() {
      this.form.altbody = null;
    },

    onShowHeaders() {
      this.isHeadersVisible = !this.isHeadersVisible;
    },

    onShowAttachField() {
      this.isAttachFieldVisible = true;
      this.$nextTick(() => {
        if (this.$refs.media && typeof this.$refs.media.focus === 'function') {
          this.$refs.media.focus();
        }
      });
    },

    onOpenAttach() {
      this.isAttachModalOpen = true;
    },

    onAttachSelect(o) {
      if (this.form.media.some((m) => m.id === o.id)) {
        return;
      }

      this.form.media.push(o);
    },

    isUnsaved() {
      return this.data.body !== this.form.content.body
        || this.data.contentType !== this.form.content.contentType;
    },

    onTab(tab) {
      this.activeTab = tab;

      if (tab === 'content' && window.tinymce?.activeEditor) {
        this.$nextTick(() => {
          window.tinymce.activeEditor.focus();
        });
      }

      // this.$router.replace({ hash: `#${tab}` });
      window.history.replaceState({}, '', `#${tab}`);
    },

    syncActiveTabWithRouteHash() {
      const requestedTab = String(this.$route.hash || '').replace('#', '');
      const validTabs = ['campaign', 'content', 'attribs', 'archive'];

      if (!validTabs.includes(requestedTab)) {
        return;
      }

      if (this.isNew && requestedTab !== 'campaign') {
        return;
      }

      this.activeTab = requestedTab;
    },

    onFillArchiveMeta() {
      const archiveStr = `{"email": "email@domain.com", "name": "${this.$t('globals.fields.name')}", "attribs": {}}`;
      this.form.archiveMetaStr = this.$utils.getPref('campaign.archiveMetaStr') || JSON.stringify(JSON.parse(archiveStr), null, 4);
    },

    onSubmit(typ) {
      // Validate custom JSON headers.
      if (this.form.headersStr && this.form.headersStr !== '[]') {
        try {
          this.form.headers = JSON.parse(this.form.headersStr);
        } catch (e) {
          this.$utils.toast(e.toString(), 'is-danger');
          return;
        }
      } else {
        this.form.headers = [];
      }

      // Validate archive JSON body.
      if (this.form.archive && this.form.archiveMetaStr) {
        try {
          this.form.archiveMeta = JSON.parse(this.form.archiveMetaStr);
        } catch (e) {
          this.$utils.toast(e.toString(), 'is-danger');
          return;
        }
      }

      // Validate custom JSON attribs.
      let attribs = null;
      if (this.form.attribsStr && this.form.attribsStr.trim()) {
        try {
          attribs = JSON.parse(this.form.attribsStr);
        } catch (e) {
          this.$utils.toast(
            `${this.$t('subscribers.invalidJSON')}: ${e.toString()}`,
            'is-danger',

            3000,
          );
          return;
        }
      }
      this.form.attribs = attribs;

      switch (typ) {
        case 'create':
          this.createCampaign();
          break;
        case 'test':
          this.sendTest();
          break;
        default:
          this.updateCampaign();
          break;
      }
    },

    getCampaign(id) {
      return this.$api.getCampaign(id).then((data) => {
        const nextForm = {
          ...this.form,
          ...data,
          fromEmail: data.fromEmail || this.form.fromEmail || this.serverConfig.from_email,
          archiveSlug: data.archiveSlug || '',
          headersStr: JSON.stringify(data.headers, null, 4),
          archiveMetaStr: data.archiveMeta ? JSON.stringify(data.archiveMeta, null, 4) : '{}',
          attribsStr: data.attribs ? JSON.stringify(data.attribs, null, 4) : '{}',

          // The structure that is populated by editor input event.
          content: {
            contentType: data.contentType,
            body: data.body,
            bodySource: data.bodySource,
            templateId: data.templateId,
          },
        };
        nextForm.media = nextForm.media.map((f) => {
          if (!f.id) {
            return { ...f, filename: `❌ ${f.filename}` };
          }
          return f;
        });

        this.form = nextForm;
        this.isAttachFieldVisible = this.form.media.length > 0;
        this.data = data;
      });
    },

    sendTest() {
      const data = {
        id: this.data.id,
        record_id: this.data.recordId || this.data.record_id,
        name: this.form.name,
        subject: this.form.subject,
        lists: this.form.lists.map((l) => l.id),
        from_email: this.form.fromEmail,
        messenger: this.form.messenger,
        type: 'regular',
        headers: this.form.headers,
        tags: this.form.tags,
        template_id: this.form.content.templateId,
        content_type: this.form.content.contentType,
        body: this.form.content.body,
        altbody: this.form.content.contentType !== 'plain' ? this.form.altbody : null,
        subscribers: this.form.testEmails,
        media: this.form.media.map((m) => m.id),
      };

      this.$api.testCampaign(data).then(() => {
        this.$utils.toast(this.$t('campaigns.testSent'));
      });
      return false;
    },

    createCampaign() {
      const data = {
        archiveSlug: this.form.subject,
        name: this.form.name,
        subject: this.form.subject,
        lists: this.form.lists.map((l) => l.id),
        list_record_ids: this.form.lists
          .map((l) => l.recordId || l.record_id)
          .filter((id) => typeof id === 'string' && id.length > 0),
        from_email: this.form.fromEmail,
        content_type: this.form.content.contentType,
        messenger: this.form.messenger,
        type: 'regular',
        tags: this.form.tags,
        send_at: this.form.sendLater ? this.form.sendAtDate : null,
        headers: this.form.headers,
        attribs: this.form.attribs,
        media: this.form.media.map((m) => m.id),
      };

      this.$api.createCampaign(data).then((d) => {
        this.$router.push({ name: 'campaign', hash: '#content', params: { id: d.recordId || d.record_id || d.id } });
      });
      return false;
    },

    async updateCampaign(typ) {
      const data = {
        archive_slug: this.form.archiveSlug,
        name: this.form.name,
        subject: this.form.subject,
        lists: this.form.lists.map((l) => l.id),
        list_record_ids: this.form.lists
          .map((l) => l.recordId || l.record_id)
          .filter((id) => typeof id === 'string' && id.length > 0),
        from_email: this.form.fromEmail,
        messenger: this.form.messenger,
        type: 'regular',
        tags: this.form.tags,
        send_at: this.form.sendLater ? this.form.sendAtDate : null,
        headers: this.form.headers,
        attribs: this.form.attribs,
        template_id: this.form.content.templateId,
        content_type: this.form.content.contentType,
        body: this.form.content.body,
        body_source: this.form.content.bodySource,
        altbody: this.form.content.contentType !== 'plain' ? this.form.altbody : null,
        archive: this.form.archive,
        archive_template_id: this.form.archiveTemplateId,
        archive_meta: this.form.archiveMeta,
        media: this.form.media.map((m) => m.id),
      };

      let typMsg = 'globals.messages.updated';
      if (typ === 'start') {
        typMsg = 'campaigns.started';
      }

      if (!this.form.sendAtDate) {
        this.form.sendLater = false;
      }

      // This promise is used by startCampaign to first save before starting.
      return new Promise((resolve) => {
        this.$api.updateCampaign(this.data.recordId || this.data.record_id || this.data.id, data).then((d) => {
          this.data = d;
          this.form.archiveSlug = d.archiveSlug;
          this.form.attribsStr = d.attribs ? JSON.stringify(d.attribs, null, 4) : '{}';

          this.$utils.toast(this.$t(typMsg, { name: d.name }));
          resolve();
        });
      });
    },

    onUpdateCampaignArchive() {
      if (this.isEditing && this.canEdit) {
        return;
      }

      const data = {
        archive: this.form.archive,
        archive_template_id: this.form.archiveTemplateId,
        archive_meta: JSON.parse(this.form.archiveMetaStr),
        archive_slug: this.form.archiveSlug,
      };

      this.$api.updateCampaignArchive(this.data.recordId || this.data.record_id || this.data.id, data).then((d) => {
        this.form.archiveSlug = d.archiveSlug;
      });
    },

    // Starts or schedule a campaign.
    startCampaign() {
      if (!this.canStart && !this.canSchedule) {
        return;
      }

      this.$utils.confirm(
        null,
        () => {
          // First save the campaign.
          this.updateCampaign().then(() => {
            // Then start/schedule it.
            let status = '';
            if (this.canStart) {
              status = 'running';
            } else if (this.canSchedule) {
              status = 'scheduled';
            } else {
              return;
            }

            this.$api.changeCampaignStatus(this.data.recordId || this.data.record_id || this.data.id, status).then(() => {
              this.$router.push({ name: 'campaigns' });
            });
          });
        },
      );
    },

    unscheduleCampaign() {
      this.$api.changeCampaignStatus(this.data.recordId || this.data.record_id || this.data.id, 'draft').then((d) => {
        this.data = d;
      });
    },
  },

  computed: {
    ...mapState(['serverConfig', 'loading', 'lists', 'templates']),

    canManage() {
      return this.$can('campaigns:manage_all', 'campaigns:manage');
    },

    canEdit() {
      return this.isNew
        || this.data.status === 'draft' || this.data.status === 'scheduled' || this.data.status === 'paused';
    },

    canSchedule() {
      return (this.data.status === 'draft' || this.data.status === 'paused') && (this.form.sendLater && this.form.sendAtDate);
    },

    canUnSchedule() {
      return this.data.status === 'scheduled';
    },

    canStart() {
      return (this.data.status === 'draft' || this.data.status === 'paused') && !this.form.sendLater;
    },

    canArchive() {
      return this.data.status !== 'cancelled' && this.data.type !== 'optin';
    },

    selectedLists() {
      if (this.selListIDs.length === 0 || !this.lists.results) {
        return [];
      }

      return this.availableLists.filter((l) => (
        this.selListIDs.indexOf(l.listValue) > -1
      ));
    },

    availableLists() {
      return Array.isArray(this.lists && this.lists.results)
        ? this.lists.results.map((list) => ({
          ...list,
          listValue: typeof list.record_id === 'string' && list.record_id.length > 0
            ? list.record_id
            : String(list.id),
        }))
        : [];
    },

    selectedListIds() {
      return Array.isArray(this.form.lists)
        ? this.form.lists.map((list) => (
          typeof list.record_id === 'string' && list.record_id.length > 0
            ? list.record_id
            : String(list.id)
        ))
        : [];
    },

    availableMessengers() {
      const messengers = this.serverConfig && Array.isArray(this.serverConfig.messengers)
        ? this.serverConfig.messengers
        : [];

      return messengers.length > 0 ? messengers : ['email'];
    },

    contentTypeOptions() {
      return Object.entries(this.contentTypes).map(([value, title]) => ({ value, title }));
    },

    tagsInput: {
      get() {
        return Array.isArray(this.form.tags) ? this.form.tags.join(', ') : '';
      },
      set(value) {
        this.form.tags = value
          .split(',')
          .map((tag) => tag.trim())
          .filter(Boolean);
      },
    },

    testEmailsInput: {
      get() {
        return Array.isArray(this.form.testEmails) ? this.form.testEmails.join(', ') : '';
      },
      set(value) {
        this.form.testEmails = value
          .split(/[\n,]/)
          .map((email) => email.trim())
          .filter(Boolean)
          .filter((email, index, all) => all.indexOf(email) === index);
      },
    },

    archiveTemplates() {
      return this.templates.filter((t) => t.type === 'campaign');
    },

    editorKey() {
      return `${this.data.id || 'new'}-${this.form.content.contentType}-${this.form.content.templateId || 'none'}`;
    },
  },

  beforeRouteLeave() {
    if (this.isUnsaved()) {
      return new Promise((resolve) => {
        this.$utils.confirm(
          this.$t('globals.messages.confirmDiscard'),
          () => resolve(true),
          () => resolve(false),
        );
      });
    }
    return true;
  },

  watch: {
    selectedLists() {
      this.form.lists = this.selectedLists;
    },

    // eslint-disable-next-line func-names
    'data.sendAt': function () {
      if (this.data.sendAt !== null) {
        this.form.sendLater = true;
        this.form.sendAtDate = dayjs(this.data.sendAt).toDate();
      } else {
        this.form.sendLater = false;
        this.form.sendAtDate = null;
      }
    },
  },

  mounted() {
    window.onbeforeunload = () => this.isUnsaved() || null;

    // Fill default form fields.
    this.form.fromEmail = this.serverConfig.from_email;

    // New campaign.
    const { id } = this.$route.params;
    if (id === 'new') {
      this.isNew = true;

      if (this.$route.query.list_id) {
        // Multiple list_id query params.
        let strIds = [];
        if (typeof this.$route.query.list_id === 'object') {
          strIds = this.$route.query.list_id;
        } else {
          strIds = [this.$route.query.list_id];
        }

        this.selListIDs = strIds
          .map((v) => parseInt(v, 10))
          .filter((v) => !Number.isNaN(v))
          .map((v) => String(v));
      }

      if (this.$route.query.list_record_id) {
        let recordIds = [];
        if (typeof this.$route.query.list_record_id === 'object') {
          recordIds = this.$route.query.list_record_id;
        } else {
          recordIds = [this.$route.query.list_record_id];
        }

        this.selListIDs = [...this.selListIDs, ...recordIds.filter((v) => typeof v === 'string' && v.length > 0)];
      }
    } else {
      if (typeof id !== 'string' || id.trim() === '') {
        this.$utils.toast(this.$t('campaigns.invalid'));
        return;
      }

      this.isEditing = true;
    }

    // Get templates list.
    this.$api.getTemplates().then((data) => {
      if (data.length > 0) {
        if (!this.form.content.templateId) {
          const tpl = data.find((i) => i.isDefault === true);
          if (tpl) {
            this.form.content.templateId = tpl.id;
          }
        }
      }
    });

    // Fetch campaign.
    if (this.isEditing) {
      this.syncActiveTabWithRouteHash();
      this.getCampaign(id).then(() => {
        this.syncActiveTabWithRouteHash();
      });
    } else {
      this.form.messenger = 'email';
    }

    this.$nextTick(() => {
      if (this.$refs.focus && typeof this.$refs.focus.focus === 'function') {
        this.$refs.focus.focus();
      }
    });

    this.$events.$on('campaign.update', () => {
      this.onSubmit('update');
    });
  },

  beforeUnmount() {
    this.$events.$off('campaign.update');
  },
};
</script>

<style scoped>

.campaign-header {
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
  padding-bottom: 16px;
}

.action-btn {
  min-width: 170px;
}

.campaign-tabs {
  border-bottom: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}

.inline-meta {
  color: rgba(var(--v-theme-on-surface), 0.62);
  font-size: 0.8rem;
}

.form-help {
  color: rgba(var(--v-theme-on-surface), 0.68);
  font-size: 0.875rem;
  margin-top: 6px;
}

.attachment-card {
  border-style: dashed;
}

.templating-link {
  color: rgb(var(--v-theme-primary));
  text-decoration: none;
}

.templating-link:hover {
  text-decoration: underline;
}

@media (max-width: 959px) {
  .campaign-actions {
    margin-top: 12px;
  }

  .action-btn {
    width: 100%;
  }
}
</style>
