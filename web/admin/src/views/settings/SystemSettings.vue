<template>
  <div class="settings-page" v-loading="loading">
    <!-- Basic Settings -->
    <el-card class="animate-in settings-card">
      <template #header>
        <div class="card-header">
          <div>
            <span class="card-title">基本设置</span>
            <span class="card-desc">控制注册和登录</span>
          </div>
          <el-button type="primary" @click="handleSaveBasic" :loading="savingBasic">保存</el-button>
        </div>
      </template>

      <el-form label-width="120px" class="settings-form">
        <el-form-item label="网站标题">
          <el-input v-model="form.site_title" placeholder="FRP Panel" />
          <span class="form-hint">显示在侧边栏和浏览器标题</span>
        </el-form-item>
        <el-form-item label="开放注册">
          <el-switch v-model="form.registration_enabled" active-value="true" inactive-value="false" />
          <span class="form-hint">关闭后新用户无法注册</span>
        </el-form-item>
        <el-form-item label="开放登录">
          <el-switch v-model="form.login_enabled" active-value="true" inactive-value="false" />
          <span class="form-hint">关闭后所有用户无法登录（管理员不受影响）</span>
        </el-form-item>
        <el-form-item label="邮箱验证注册">
          <el-switch v-model="form.email_verification_enabled" active-value="true" inactive-value="false" />
          <span class="form-hint">开启后注册需要输入邮箱验证码（需先配置 SMTP）</span>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- SMTP Settings -->
    <el-card class="animate-in animate-in-delay-1 settings-card">
      <template #header>
        <div class="card-header">
          <div>
            <span class="card-title">邮件设置</span>
            <span class="card-desc">配置 SMTP 服务器用于发送验证码和通知</span>
          </div>
          <div class="card-header-actions">
            <el-button @click="handleTestSMTP" :loading="testingSMTP" :disabled="!form.smtp_host">发送测试邮件</el-button>
            <el-button type="primary" @click="handleSaveSMTP" :loading="savingSMTP">保存</el-button>
          </div>
        </div>
      </template>

      <el-form label-width="120px" class="settings-form">
        <el-row :gutter="20">
          <el-col :span="16">
            <el-form-item label="SMTP 服务器">
              <el-input v-model="form.smtp_host" placeholder="smtp.qq.com" />
            </el-form-item>
          </el-col>
          <el-col :span="8">
            <el-form-item label="端口">
              <el-input v-model="form.smtp_port" placeholder="587" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="用户名">
              <el-input v-model="form.smtp_user" placeholder="your@email.com" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="密码">
              <el-input v-model="form.smtp_password" type="password" show-password placeholder="SMTP 授权码" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item label="发件人地址">
          <el-input v-model="form.smtp_from" placeholder="your@email.com" />
          <span class="form-hint">部分邮箱要求发件人与用户名一致</span>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Free Plan Settings -->
    <el-card class="animate-in animate-in-delay-2 settings-card">
      <template #header>
        <div class="card-header">
          <div>
            <span class="card-title">免费版设置</span>
            <span class="card-desc">未购买套餐的用户默认使用此配额</span>
          </div>
          <el-button type="primary" @click="handleSaveFreePlan" :loading="savingFreePlan">保存</el-button>
        </div>
      </template>

      <el-form label-width="120px" class="settings-form">
        <el-form-item label="代理数量">
          <el-input v-model="form.free_max_proxies" placeholder="5" style="width: 200px">
            <template #append>个</template>
          </el-input>
        </el-form-item>
        <el-form-item label="带宽限制">
          <el-input v-model="form.free_max_bandwidth_mb" placeholder="10" style="width: 200px">
            <template #append>MB/s</template>
          </el-input>
        </el-form-item>
        <el-form-item label="流量限额">
          <el-input v-model="form.free_max_traffic_gb" placeholder="10" style="width: 200px">
            <template #append>GB/月</template>
          </el-input>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- Invite Rebate Settings -->
    <el-card class="animate-in animate-in-delay-2 settings-card">
      <template #header>
        <div class="card-header">
          <div>
            <span class="card-title">邀请返利设置</span>
            <span class="card-desc">被邀请用户完成付费订单时，按订单金额百分比返还余额给邀请人</span>
          </div>
          <el-button type="primary" @click="handleSaveRebate" :loading="savingRebate">保存</el-button>
        </div>
      </template>

      <el-form label-width="120px" class="settings-form">
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="一级返利">
              <el-input v-model="form.invite_rebate_level1_percent" placeholder="10">
                <template #append>%</template>
              </el-input>
              <span class="form-hint">直推下级每单返利比例</span>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="二级返利">
              <el-input v-model="form.invite_rebate_level2_percent" placeholder="5">
                <template #append>%</template>
              </el-input>
              <span class="form-hint">下级的下级每单返利比例</span>
            </el-form-item>
          </el-col>
        </el-row>
      </el-form>
    </el-card>

    <!-- Test SMTP Dialog -->
    <el-dialog v-model="showTestDialog" title="发送测试邮件" width="400">
      <el-form label-width="0">
        <el-form-item>
          <el-input v-model="testEmail" placeholder="输入接收测试邮件的邮箱地址" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showTestDialog = false">取消</el-button>
        <el-button type="primary" @click="doTestSMTP" :loading="testingSMTP">发送</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { getSettings, updateSettings, testSMTP } from '../../api'

const loading = ref(false)
const savingBasic = ref(false)
const savingSMTP = ref(false)
const savingRebate = ref(false)
const savingFreePlan = ref(false)
const testingSMTP = ref(false)
const showTestDialog = ref(false)
const testEmail = ref('')

const form = reactive({
  registration_enabled: 'true',
  login_enabled: 'true',
  site_title: 'FRP Panel',
  email_verification_enabled: 'false',
  smtp_host: '',
  smtp_port: '587',
  smtp_user: '',
  smtp_password: '',
  smtp_from: '',
  invite_rebate_level1_percent: '10',
  invite_rebate_level2_percent: '5',
  free_max_proxies: '',
  free_max_bandwidth_mb: '',
  free_max_traffic_gb: '',
})

const freePlanDefaults = {
  free_max_proxies: '5',
  free_max_bandwidth_mb: '10',
  free_max_traffic_gb: '10',
}

onMounted(() => fetchData())

async function fetchData() {
  loading.value = true
  try {
    const res = await getSettings()
    const data = res.data || {}
    Object.keys(form).forEach(key => {
      if (data[key] !== undefined) (form as any)[key] = data[key]
    })
    // Apply defaults for free plan fields if not set
    for (const [key, val] of Object.entries(freePlanDefaults)) {
      if (!data[key]) (form as any)[key] = val
    }
  } finally {
    loading.value = false
  }
}

async function handleSaveBasic() {
  savingBasic.value = true
  try {
    await updateSettings({
      site_title: form.site_title,
      registration_enabled: form.registration_enabled,
      login_enabled: form.login_enabled,
      email_verification_enabled: form.email_verification_enabled,
    })
    ElMessage.success('基本设置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '保存失败')
  } finally {
    savingBasic.value = false
  }
}

async function handleSaveSMTP() {
  savingSMTP.value = true
  try {
    const data: Record<string, string> = {
      smtp_host: form.smtp_host,
      smtp_port: form.smtp_port,
      smtp_user: form.smtp_user,
      smtp_from: form.smtp_from,
    }
    if (form.smtp_password && form.smtp_password !== '********') {
      data.smtp_password = form.smtp_password
    }
    await updateSettings(data)
    ElMessage.success('邮件设置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '保存失败')
  } finally {
    savingSMTP.value = false
  }
}

async function handleSaveRebate() {
  savingRebate.value = true
  try {
    await updateSettings({
      invite_rebate_level1_percent: form.invite_rebate_level1_percent,
      invite_rebate_level2_percent: form.invite_rebate_level2_percent,
    })
    ElMessage.success('返利设置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '保存失败')
  } finally {
    savingRebate.value = false
  }
}

async function handleSaveFreePlan() {
  savingFreePlan.value = true
  try {
    await updateSettings({
      free_max_proxies: form.free_max_proxies,
      free_max_bandwidth_mb: form.free_max_bandwidth_mb,
      free_max_traffic_gb: form.free_max_traffic_gb,
    })
    ElMessage.success('免费版设置已保存')
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '保存失败')
  } finally {
    savingFreePlan.value = false
  }
}

function handleTestSMTP() {
  testEmail.value = ''
  showTestDialog.value = true
}

async function doTestSMTP() {
  if (!testEmail.value) {
    ElMessage.error('请输入邮箱地址')
    return
  }
  testingSMTP.value = true
  try {
    await testSMTP({ to: testEmail.value })
    ElMessage.success('测试邮件已发送，请检查收件箱')
    showTestDialog.value = false
  } catch (e: any) {
    ElMessage.error(e.response?.data?.message || '发送失败')
  } finally {
    testingSMTP.value = false
  }
}
</script>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.settings-card {
  /* no extra styles needed */
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.card-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--color-text);
  display: block;
}

.card-desc {
  font-size: 12px;
  color: var(--color-text-muted);
  margin-top: 2px;
  display: block;
}

.card-header-actions {
  display: flex;
  gap: 8px;
}

.settings-form {
  max-width: 640px;
}

.form-hint {
  font-size: 12px;
  color: var(--color-text-muted);
  margin-left: 12px;
}
</style>
