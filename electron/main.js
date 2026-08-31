const { app, BrowserWindow, dialog, Tray, Menu, shell } = require('electron');
const { spawn } = require('child_process');
const crypto = require('crypto');
const path = require('path');
const http = require('http');
const fs = require('fs');
const {
  DEFAULT_BIND_ADDRESS,
  isLoopbackAddress,
  isPrivateAddress,
  getPrivateIPv4Candidates,
  buildRelaunchArgs,
  resolveRuntimeConfig,
  describeRuntimeConfig,
  getHealthCheckAddress,
  isSuccessfulHttpStatus,
  isSuccessfulStatusResponse,
} = require('./runtime-config');

const APP_NAME = 'MyAPI';
const CANONICAL_BINARY_NAME = process.platform === 'win32' ? 'my-api.exe' : 'my-api';
const LEGACY_BINARY_NAME = process.platform === 'win32' ? 'new-api.exe' : 'new-api';
const CANONICAL_DATABASE_NAME = 'my-api.db';
const LEGACY_DATABASE_NAMES = ['new-api.db', 'one-api.db'];

const DESKTOP_ARGS = process.argv.slice(1);

let mainWindow;
let serverProcess;
let tray = null;
let serverErrorLogs = [];
const DEV_FRONTEND_PORT = 5173; // Rsbuild dev server port
// Desktop/LAN is loopback-only by default. An explicit --allow-lan launch may
// opt into a private-network bind; the backend receives the validated address.
let runtimeConfig;
try {
  runtimeConfig = resolveRuntimeConfig({ args: DESKTOP_ARGS, env: process.env });
} catch (error) {
  // Fail before creating a window or spawning a server, making unsafe launch
  // parameters visible to users and automation alike.
  console.error(`Invalid MyAPI desktop configuration: ${error.message}`);
  runtimeConfig = { bindAddress: '127.0.0.1', port: 3000, allowLan: false, isLan: false, error };
}
const PORT = runtimeConfig.port;
const BIND_ADDRESS = runtimeConfig.bindAddress;
const ALLOW_LAN = runtimeConfig.allowLan;

const hasSingleInstanceLock = app.requestSingleInstanceLock();
if (!hasSingleInstanceLock) {
  app.quit();
} else {
  app.on('second-instance', () => {
    if (!mainWindow) return;
    if (mainWindow.isMinimized()) mainWindow.restore();
    mainWindow.show();
    mainWindow.focus();
  });
}

function ensureSessionSecret(userDataPath) {
  const secretPath = path.join(userDataPath, 'myapi-session-secret');
  try {
    if (fs.existsSync(secretPath)) {
      const existing = fs.readFileSync(secretPath, 'utf8').trim();
      if (/^[a-f0-9]{64}$/i.test(existing)) return existing;
    }
    const generated = crypto.randomBytes(32).toString('hex');
    fs.writeFileSync(secretPath, `${generated}\n`, { mode: 0o600 });
    try { fs.chmodSync(secretPath, 0o600); } catch (_) { /* Windows ACLs apply */ }
    return generated;
  } catch (error) {
    throw new Error(`无法保存 MyAPI 会话密钥: ${error.message}`);
  }
}

// 保存日志到文件并打开
function saveAndOpenErrorLog() {
  try {
    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
    const logFileName = `my-api-crash-${timestamp}.log`;
    const logDir = app.getPath('logs');
    const logFilePath = path.join(logDir, logFileName);
    
    // 确保日志目录存在
    if (!fs.existsSync(logDir)) {
      fs.mkdirSync(logDir, { recursive: true });
    }
    
    // 写入日志
    const logContent = `${APP_NAME} 崩溃日志
生成时间: ${new Date().toLocaleString('zh-CN')}
平台: ${process.platform}
架构: ${process.arch}
应用版本: ${app.getVersion()}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

完整错误日志:

${serverErrorLogs.join('\n')}

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

日志文件位置: ${logFilePath}
`;
    
    fs.writeFileSync(logFilePath, logContent, 'utf8');
    
    // 打开日志文件
    shell.openPath(logFilePath).then((error) => {
      if (error) {
        console.error('Failed to open log file:', error);
        // 如果打开文件失败，至少显示文件位置
        shell.showItemInFolder(logFilePath);
      }
    });
    
    return logFilePath;
  } catch (err) {
    console.error('Failed to save error log:', err);
    return null;
  }
}

// 分析错误日志，识别常见错误并提供解决方案
function analyzeError(errorLogs) {
  const allLogs = errorLogs.join('\n');
  
  // 检测端口占用错误
  if (allLogs.includes('failed to start HTTP server') || 
      allLogs.includes('bind: address already in use') ||
      allLogs.includes('listen tcp') && allLogs.includes('bind: address already in use')) {
    return {
      type: '端口被占用',
      title: '端口 ' + PORT + ' 被占用',
      message: '无法启动服务器，端口已被其他程序占用',
      solution: `可能的解决方案：\n\n1. 关闭占用端口 ${PORT} 的其他程序\n2. 检查是否已经运行了另一个 ${APP_NAME} 实例\n3. 使用以下命令查找占用端口的进程：\n   Mac/Linux: lsof -i :${PORT}\n   Windows: netstat -ano | findstr :${PORT}\n4. 重启电脑以释放端口`
    };
  }
  
  // 检测数据库错误
  if (allLogs.includes('database is locked') || 
      allLogs.includes('unable to open database')) {
    return {
      type: '数据文件被占用',
      title: '无法访问数据文件',
      message: '应用的数据文件正被其他程序占用',
      solution: `可能的解决方案：\n\n1. 检查是否已经打开了另一个 ${APP_NAME} 窗口\n   - 查看任务栏/Dock 中是否有其他 ${APP_NAME} 图标\n   - 查看系统托盘（Windows）或菜单栏（Mac）中是否有 ${APP_NAME} 图标\n\n2. 如果刚刚关闭过应用，请等待 10 秒后再试\n\n3. 重启电脑以释放被占用的文件\n\n4. 如果问题持续，可以尝试：\n   - 退出所有 ${APP_NAME} 实例\n   - 删除数据目录中的临时文件（.db-shm 和 .db-wal）\n   - 重新启动应用`
    };
  }
  
  // 检测权限错误
  if (allLogs.includes('permission denied') || 
      allLogs.includes('access denied')) {
    return {
      type: '权限错误',
      title: '权限不足',
      message: '程序没有足够的权限执行操作',
      solution: '可能的解决方案：\n\n1. 以管理员/root权限运行程序\n2. 检查数据目录的读写权限\n3. 检查可执行文件的权限\n4. 在 Mac 上，检查安全性与隐私设置'
    };
  }
  
  // 检测网络错误
  if (allLogs.includes('network is unreachable') || 
      allLogs.includes('no such host') ||
      allLogs.includes('connection refused')) {
    return {
      type: '网络错误',
      title: '网络连接失败',
      message: '无法建立网络连接',
      solution: '可能的解决方案：\n\n1. 检查网络连接是否正常\n2. 检查防火墙设置\n3. 检查代理配置\n4. 确认目标服务器地址正确'
    };
  }
  
  // 检测配置文件错误
  if (allLogs.includes('invalid configuration') || 
      allLogs.includes('failed to parse config') ||
      allLogs.includes('yaml') || allLogs.includes('json') && allLogs.includes('parse')) {
    return {
      type: '配置错误',
      title: '配置文件错误',
      message: '配置文件格式不正确或包含无效配置',
      solution: '可能的解决方案：\n\n1. 检查配置文件格式是否正确\n2. 恢复默认配置\n3. 删除配置文件让程序重新生成\n4. 查看文档了解正确的配置格式'
    };
  }
  
  // 检测内存不足
  if (allLogs.includes('out of memory') || 
      allLogs.includes('cannot allocate memory')) {
    return {
      type: '内存不足',
      title: '系统内存不足',
      message: '程序运行时内存不足',
      solution: '可能的解决方案：\n\n1. 关闭其他占用内存的程序\n2. 增加系统可用内存\n3. 重启电脑释放内存\n4. 检查是否存在内存泄漏'
    };
  }
  
  // 检测文件不存在错误
  if (allLogs.includes('no such file or directory') || 
      allLogs.includes('cannot find the file')) {
    return {
      type: '文件缺失',
      title: '找不到必需的文件',
      message: '缺少程序运行所需的文件',
      solution: '可能的解决方案：\n\n1. 重新安装应用程序\n2. 检查安装目录是否完整\n3. 确保所有依赖文件都存在\n4. 检查文件路径是否正确'
    };
  }
  
  return null;
}

function getBinaryPath() {
  const isDev = process.env.NODE_ENV === 'development';
  const platform = process.platform;

  if (isDev) {
    const canonicalPath = path.join(__dirname, '..', CANONICAL_BINARY_NAME);
    const legacyPath = path.join(__dirname, '..', LEGACY_BINARY_NAME);
    return fs.existsSync(canonicalPath) ? canonicalPath : legacyPath;
  }

  const canonicalPath = path.join(process.resourcesPath, 'bin', CANONICAL_BINARY_NAME);
  const legacyPath = path.join(process.resourcesPath, 'bin', LEGACY_BINARY_NAME);
  return fs.existsSync(canonicalPath) ? canonicalPath : legacyPath;
}

// Check if a server is available with retry logic
function checkServerAvailability(port, maxRetries = 30, retryDelay = 1000, hostname = '127.0.0.1', requestPath = '/') {
  return new Promise((resolve, reject) => {
    let currentAttempt = 0;
    
    const tryConnect = () => {
      currentAttempt++;
      
      if (currentAttempt % 5 === 1 && currentAttempt > 1) {
        console.log(`Attempting to connect to port ${port}... (attempt ${currentAttempt}/${maxRetries})`);
      }
      
      const req = http.get({
        // Probe the effective listener.  A private LAN bind is not necessarily
        // reachable through loopback, while 0.0.0.0 remains probeable via IPv4
        // loopback on supported platforms.
        hostname: getHealthCheckAddress(hostname),
        port,
        path: requestPath,
        timeout: 10000
      }, (res) => {
        const statusCode = Number(res.statusCode || 0);
        let responseBody = '';
        if (requestPath === '/api/status') {
          res.setEncoding('utf8');
          res.on('data', (chunk) => {
            if (responseBody.length < 65536) responseBody += chunk;
          });
        } else {
          res.resume();
        }
        res.on('end', () => {
          const successful = requestPath === '/api/status'
            ? isSuccessfulStatusResponse(statusCode, responseBody)
            : isSuccessfulHttpStatus(statusCode);
          if (successful) {
            console.log(`✓ Successfully connected to ${requestPath} on port ${port} (status: ${statusCode})`);
            resolve();
            return;
          }
          if (currentAttempt >= maxRetries) {
            reject(new Error(`Unexpected HTTP response from ${requestPath} on port ${port} after ${maxRetries} attempts`));
          } else {
            setTimeout(tryConnect, retryDelay);
          }
        });
      });

      req.on('error', (err) => {
        if (currentAttempt >= maxRetries) {
          reject(new Error(`Failed to connect to port ${port} after ${maxRetries} attempts: ${err.message}`));
        } else {
          setTimeout(tryConnect, retryDelay);
        }
      });

      req.on('timeout', () => {
        req.destroy();
        if (currentAttempt >= maxRetries) {
          reject(new Error(`Connection timeout on port ${port} after ${maxRetries} attempts`));
        } else {
          setTimeout(tryConnect, retryDelay);
        }
      });
    };
    
    tryConnect();
  });
}

function startServer() {
  return new Promise((resolve, reject) => {
    const isDev = process.env.NODE_ENV === 'development';

    if (!isLoopbackAddress(BIND_ADDRESS) && !ALLOW_LAN) {
      reject(new Error('LAN sharing is disabled by default; restart with --allow-lan and a private bind address.'));
      return;
    }
    if (!isLoopbackAddress(BIND_ADDRESS) && !isPrivateAddress(BIND_ADDRESS)) {
      reject(new Error('Desktop LAN binding must use a private IPv4 address or 0.0.0.0.'));
      return;
    }

    const userDataPath = app.getPath('userData');
    const dataDir = path.join(userDataPath, 'data');
    // Packaged apps may run from a read-only resources directory (notably
    // Windows Program Files). Keep backend and full-content logs under the
    // per-user writable data root instead of relying on the process cwd.
    const logsDir = path.join(userDataPath, 'logs');
    
    // 设置环境变量供 preload.js 使用
    process.env.ELECTRON_DATA_DIR = dataDir;
    
    if (isDev) {
      // 开发模式：假设开发者手动启动了 Go 后端和前端开发服务器
      // 只需要等待前端开发服务器就绪
      console.log('Development mode: skipping server startup');
      console.log('Please make sure you have started:');
      console.log('  1. Go backend: go run main.go (port 3000)');
      console.log('  2. Frontend dev server: make dev-web (port 5173)');
      console.log('');
      console.log('Checking if servers are running...');
      
      // First check if both servers are accessible
      // The dev server owns `/`; it may not proxy the backend status endpoint.
      checkServerAvailability(DEV_FRONTEND_PORT, 30, 1000, '127.0.0.1', '/')
        .then(() => {
          console.log('✓ Frontend dev server is accessible on port 5173');
          resolve();
        })
        .catch((err) => {
          console.error(`✗ Cannot connect to frontend dev server on port ${DEV_FRONTEND_PORT}`);
          console.error('Please make sure the frontend dev server is running:');
          console.error('  make dev-web');
          reject(err);
        });
      return;
    }

    // 生产模式：启动二进制服务器
    const env = {
      ...process.env,
      PORT: PORT.toString(),
      MYAPI_BIND_ADDRESS: BIND_ADDRESS,
      MYAPI_EDITION: 'lan',
      SESSION_COOKIE_SECURE: 'false',
      FULL_CONTENT_LOG_DIR: path.join(logsDir, 'full-content'),
    };

    if (!fs.existsSync(dataDir)) {
      fs.mkdirSync(dataDir, { recursive: true });
    }
    if (!fs.existsSync(logsDir)) {
      fs.mkdirSync(logsDir, { recursive: true });
    }
    env.SESSION_SECRET = process.env.SESSION_SECRET || ensureSessionSecret(userDataPath);

    const canonicalDatabasePath = path.join(dataDir, CANONICAL_DATABASE_NAME);
    const legacyDatabasePath = LEGACY_DATABASE_NAMES
      .map((name) => path.join(dataDir, name))
      .find((candidate) => fs.existsSync(candidate));
    // Keep an explicitly configured path authoritative. Otherwise new data is
    // written to my-api.db, while an existing legacy database is adopted in
    // place rather than silently creating an empty account.
    env.SQLITE_PATH = process.env.MYAPI_SQLITE_PATH || process.env.SQLITE_PATH ||
      legacyDatabasePath || canonicalDatabasePath;
    
    console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
    console.log('📁 您的数据存储位置：');
    console.log('   ' + dataDir);
    console.log('   💡 备份提示：复制此目录即可备份所有数据');
    console.log('━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');

    const binaryPath = getBinaryPath();
    const workingDir = process.resourcesPath;
    
    console.log('Starting server from:', binaryPath);

    serverProcess = spawn(binaryPath, ['--log-dir', logsDir], {
      env,
      cwd: workingDir
    });

    serverProcess.stdout.on('data', (data) => {
      console.log(`Server: ${data}`);
    });

    serverProcess.stderr.on('data', (data) => {
      const errorMsg = data.toString();
      console.error(`Server Error: ${errorMsg}`);
      serverErrorLogs.push(errorMsg);
      // 只保留最近的100条错误日志
      if (serverErrorLogs.length > 100) {
        serverErrorLogs.shift();
      }
    });

    serverProcess.on('error', (err) => {
      console.error('Failed to start server:', err);
      reject(err);
    });

    serverProcess.on('close', (code) => {
      console.log(`Server process exited with code ${code}`);
      
      // 如果退出代码不是0，说明服务器异常退出
      if (code !== 0 && code !== null) {
        const errorDetails = serverErrorLogs.length > 0 
          ? serverErrorLogs.slice(-20).join('\n') 
          : '没有捕获到错误日志';
        
        // 分析错误类型
        const knownError = analyzeError(serverErrorLogs);
        
        let dialogOptions;
        if (knownError) {
          // 识别到已知错误，显示友好的错误信息和解决方案
          dialogOptions = {
            type: 'error',
            title: knownError.title,
            message: knownError.message,
            detail: `${knownError.solution}\n\n━━━━━━━━━━━━━━━━━━━━━━\n\n退出代码: ${code}\n\n错误类型: ${knownError.type}\n\n最近的错误日志:\n${errorDetails}`,
            buttons: ['退出应用', '查看完整日志'],
            defaultId: 0,
            cancelId: 0
          };
        } else {
          // 未识别的错误，显示通用错误信息
          dialogOptions = {
            type: 'error',
            title: '服务器崩溃',
            message: '服务器进程异常退出',
            detail: `退出代码: ${code}\n\n最近的错误信息:\n${errorDetails}`,
            buttons: ['退出应用', '查看完整日志'],
            defaultId: 0,
            cancelId: 0
          };
        }
        
        dialog.showMessageBox(dialogOptions).then((result) => {
          if (result.response === 1) {
            // 用户选择查看详情，保存并打开日志文件
            const logPath = saveAndOpenErrorLog();
            
            // 显示确认对话框
            const confirmMessage = logPath 
              ? `日志已保存到:\n${logPath}\n\n日志文件已在默认文本编辑器中打开。\n\n点击"退出"关闭应用程序。`
              : '日志保存失败，但已在控制台输出。\n\n点击"退出"关闭应用程序。';
            
            dialog.showMessageBox({
              type: 'info',
              title: '日志已保存',
              message: confirmMessage,
              buttons: ['退出'],
              defaultId: 0
            }).then(() => {
              app.isQuitting = true;
              app.quit();
            });
            
            // 同时在控制台输出
            console.log('=== 完整错误日志 ===');
            console.log(serverErrorLogs.join('\n'));
          } else {
            // 用户选择直接退出
            app.isQuitting = true;
            app.quit();
          }
        });
      } else {
        // 正常退出（code为0或null），直接关闭窗口
        if (mainWindow && !mainWindow.isDestroyed()) {
          mainWindow.close();
        }
      }
    });

    const healthCheckHost = getHealthCheckAddress(BIND_ADDRESS);
    // Probe the backend's public status endpoint, not merely an arbitrary
    // process that happens to occupy the configured port.
    checkServerAvailability(PORT, 30, 1000, healthCheckHost, '/api/status')
      .then(() => {
        console.log(`✓ Backend server is accessible at ${healthCheckHost}:${PORT}`);
        resolve();
      })
      .catch((err) => {
        console.error('✗ Failed to connect to backend server');
        reject(err);
      });
  });
}

function createWindow() {
  const isDev = process.env.NODE_ENV === 'development';
  const loadPort = isDev ? DEV_FRONTEND_PORT : PORT;
  // In a LAN launch the backend may bind only to its concrete private IPv4
  // address. Loading the packaged UI through hard-coded loopback would then
  // fail on both macOS and Windows, even though the health check succeeded.
  // Development keeps loopback because the separate dev server normally
  // listens there; packaged builds probe the same effective host as the
  // backend (with 0.0.0.0 safely mapped to loopback).
  const loadHost = isDev ? '127.0.0.1' : getHealthCheckAddress(BIND_ADDRESS);
  
  mainWindow = new BrowserWindow({
    width: 1080,
    height: 720,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      nodeIntegration: false,
      contextIsolation: true
    },
    title: APP_NAME,
    icon: path.join(__dirname, 'icon.png')
  });

  mainWindow.loadURL(`http://${loadHost}:${loadPort}`);
  
  console.log(`Loading from: http://${loadHost}:${loadPort}`);

  if (isDev) {
    mainWindow.webContents.openDevTools();
  }

  // Close to tray instead of quitting
  mainWindow.on('close', (event) => {
    if (!app.isQuitting) {
      event.preventDefault();
      mainWindow.hide();
      if (process.platform === 'darwin') {
        app.dock.hide();
      }
    }
  });

  mainWindow.on('closed', () => {
    mainWindow = null;
  });
}

async function confirmAndRelaunch({ bindAddress, allowLan, title, message, detail }) {
  // Validate the exact argument set that will be passed to the next process.
  // The current process is never rebound in place.
  const relaunchArgs = buildRelaunchArgs(DESKTOP_ARGS, {
    bindAddress,
    port: PORT,
    allowLan,
  });
  resolveRuntimeConfig({ args: relaunchArgs, env: {} });

  const result = await dialog.showMessageBox({
    type: 'question',
    title,
    message,
    detail,
    buttons: ['取消', '退出并重启'],
    defaultId: 1,
    cancelId: 0,
    noLink: true,
  });
  if (result.response !== 1) return;

  // Electron relaunch starts a fresh process; quitting then lets the existing
  // before-quit handler terminate the bundled backend cleanly.
  app.isQuitting = true;
  app.relaunch({ args: relaunchArgs });
  app.quit();
}

function restartWithLan() {
  const candidates = getPrivateIPv4Candidates();
  if (candidates.length === 0) {
    dialog.showMessageBox({
      type: 'warning',
      title: '未找到可用的局域网地址',
      message: 'MyAPI 没有发现可用的私有 IPv4 地址。',
      detail: '请先连接到 10.x、172.16–31.x 或 192.168.x 的局域网，再重试。不会读取或导入任何本地凭据。',
      buttons: ['关闭'],
      defaultId: 0,
    });
    return;
  }

  const selectedAddress = candidates[0];
  const addressSummary = candidates.length > 1
    ? `发现的地址：${candidates.join('、')}\n将使用：${selectedAddress}`
    : `局域网地址：${selectedAddress}`;
  confirmAndRelaunch({
    bindAddress: selectedAddress,
    allowLan: true,
    title: '重启并启用局域网共享？',
    message: 'MyAPI 将退出并重启，然后仅绑定到一个私有局域网 IPv4 地址。',
    detail: `${addressSummary}\n端口：${PORT}\n\n局域网中的同事需要使用 API Key 访问。监听地址只在启动时确定，不会动态重绑定。`,
  }).catch((error) => {
    console.error('Failed to prepare LAN relaunch:', error);
    dialog.showErrorBox('无法启用局域网共享', error.message);
  });
}

function restartLoopbackOnly() {
  confirmAndRelaunch({
    bindAddress: DEFAULT_BIND_ADDRESS,
    allowLan: false,
    title: '恢复仅本机访问？',
    message: 'MyAPI 将退出并重启，然后仅监听本机回环地址。',
    detail: `地址：${DEFAULT_BIND_ADDRESS}\n端口：${PORT}\n\n局域网中的其他设备将无法访问此实例。`,
  }).catch((error) => {
    console.error('Failed to prepare loopback relaunch:', error);
    dialog.showErrorBox('无法恢复仅本机访问', error.message);
  });
}

function createTray() {
  // Use template icon for macOS (black with transparency, auto-adapts to theme)
  // Use colored icon for Windows
  const trayIconPath = process.platform === 'darwin'
    ? path.join(__dirname, 'tray-iconTemplate.png')
    : path.join(__dirname, 'tray-icon-windows.png');

  tray = new Tray(trayIconPath);

  const trayStatus = describeRuntimeConfig(runtimeConfig, process.platform);

  const contextMenu = Menu.buildFromTemplate([
    {
      label: `Show ${APP_NAME}`,
      click: () => {
        if (mainWindow === null) {
          createWindow();
        } else {
          mainWindow.show();
          if (process.platform === 'darwin') {
            app.dock.show();
          }
        }
      }
    },
    {
      label: `Endpoint: ${trayStatus.endpoint}`,
      enabled: false,
    },
    {
      label: runtimeConfig.isLan
        ? 'LAN sharing enabled (private network)'
        : 'LAN sharing disabled (loopback only)',
      enabled: false,
    },
    {
      label: 'LAN status and connection help…',
      click: () => {
        const status = describeRuntimeConfig(runtimeConfig, process.platform);
        const mode = status.lanEnabled
          ? 'LAN sharing is enabled for a private network.'
          : 'LAN sharing is disabled; this instance is reachable only from this computer.';
        dialog.showMessageBox({
          type: 'info',
          title: 'MyAPI LAN status',
          message: mode,
          detail: [
            `Endpoint: ${status.endpoint}`,
            `Mode: ${status.mode}`,
            '',
            status.firewallHint,
            status.restartHint,
          ].join('\n'),
          buttons: ['Close'],
          defaultId: 0,
        });
      },
    },
    { type: 'separator' },
    {
      label: '重启并启用 LAN…',
      click: restartWithLan,
    },
    {
      label: '重启并恢复仅本机',
      click: restartLoopbackOnly,
    },
    { type: 'separator' },
    {
      label: 'Quit',
      click: () => {
        app.isQuitting = true;
        app.quit();
      }
    }
  ]);

  tray.setToolTip(APP_NAME);
  tray.setContextMenu(contextMenu);

  // On macOS, clicking the tray icon shows the window
  tray.on('click', () => {
    if (mainWindow === null) {
      createWindow();
    } else {
      mainWindow.isVisible() ? mainWindow.hide() : mainWindow.show();
      if (mainWindow.isVisible() && process.platform === 'darwin') {
        app.dock.show();
      }
    }
  });
}

app.whenReady().then(async () => {
  if (!hasSingleInstanceLock) return;
  if (runtimeConfig.error) {
    await dialog.showMessageBox({
      type: 'error',
      title: '启动参数无效',
      message: runtimeConfig.error.message,
      detail: '默认只监听本机回环地址。若要共享给同事，请使用 --allow-lan 并指定私有 IPv4 地址。',
      buttons: ['退出'],
    });
    app.quit();
    return;
  }
  try {
    await startServer();
    createTray();
    createWindow();
  } catch (err) {
    console.error('Failed to start application:', err);
    
    // 分析启动失败的错误
    const knownError = analyzeError(serverErrorLogs);
    
    if (knownError) {
      dialog.showMessageBox({
        type: 'error',
        title: knownError.title,
        message: `启动失败: ${knownError.message}`,
        detail: `${knownError.solution}\n\n━━━━━━━━━━━━━━━━━━━━━━\n\n错误信息: ${err.message}\n\n错误类型: ${knownError.type}`,
        buttons: ['退出', '查看完整日志'],
        defaultId: 0,
        cancelId: 0
      }).then((result) => {
        if (result.response === 1) {
          // 用户选择查看日志
          const logPath = saveAndOpenErrorLog();
          
          const confirmMessage = logPath 
            ? `日志已保存到:\n${logPath}\n\n日志文件已在默认文本编辑器中打开。\n\n点击"退出"关闭应用程序。`
            : '日志保存失败，但已在控制台输出。\n\n点击"退出"关闭应用程序。';
          
          dialog.showMessageBox({
            type: 'info',
            title: '日志已保存',
            message: confirmMessage,
            buttons: ['退出'],
            defaultId: 0
          }).then(() => {
            app.quit();
          });
          
          console.log('=== 完整错误日志 ===');
          console.log(serverErrorLogs.join('\n'));
        } else {
          app.quit();
        }
      });
    } else {
      dialog.showMessageBox({
        type: 'error',
        title: '启动失败',
        message: '无法启动服务器',
        detail: `错误信息: ${err.message}\n\n请检查日志获取更多信息。`,
        buttons: ['退出', '查看完整日志'],
        defaultId: 0,
        cancelId: 0
      }).then((result) => {
        if (result.response === 1) {
          // 用户选择查看日志
          const logPath = saveAndOpenErrorLog();
          
          const confirmMessage = logPath 
            ? `日志已保存到:\n${logPath}\n\n日志文件已在默认文本编辑器中打开。\n\n点击"退出"关闭应用程序。`
            : '日志保存失败，但已在控制台输出。\n\n点击"退出"关闭应用程序。';
          
          dialog.showMessageBox({
            type: 'info',
            title: '日志已保存',
            message: confirmMessage,
            buttons: ['退出'],
            defaultId: 0
          }).then(() => {
            app.quit();
          });
          
          console.log('=== 完整错误日志 ===');
          console.log(serverErrorLogs.join('\n'));
        } else {
          app.quit();
        }
      });
    }
  }
});

app.on('window-all-closed', () => {
  // Don't quit when window is closed, keep running in tray
  // Only quit when explicitly choosing Quit from tray menu
});

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow();
  }
});

app.on('before-quit', (event) => {
  if (serverProcess) {
    event.preventDefault();

    console.log('Shutting down server...');
    serverProcess.kill('SIGTERM');

    setTimeout(() => {
      if (serverProcess) {
        serverProcess.kill('SIGKILL');
      }
      app.exit();
    }, 5000);

    serverProcess.on('close', () => {
      serverProcess = null;
      app.exit();
    });
  }
});
