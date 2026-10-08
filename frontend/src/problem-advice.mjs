// Rules inspect the current attempt only. They suggest actions; never run commands.
export function problemAdvice(lines, status = {}) {
  if (status.recovery === 'recovered') return null;
  const texts = lines.filter(line => (line.attempt || 1) >= (status.attempt || 1))
    .slice(-500).map(line => String(line.text || '').replace(/\x1b\[[0-9;]*m/g, ''));
  if (status.error) texts.push(status.error);
  const text = texts.join('\n');
  const rule = [
    { id: 'port', pattern: /(?:Port \d+ is already in use|Address already in use|端口\s*\d+\s*已被占用)/i,
      title: '端口被其他进程占用', steps: ['点击“启动检查”查看占用进程和所属配置。', '停止对应服务或换一个端口，然后重新启动。'] },
    { id: 'java-version', pattern: /(?:UnsupportedClassVersionError|class file has wrong version|invalid (?:target release|source release)|release version \d+ not supported)/i,
      title: 'JDK 与编译目标不匹配', steps: ['对比“启动检查”中的 Java 版本与项目 pom.xml 的版本要求。', '配置兼容的 JDK；切换 JDK 后重新构建。'] },
    { id: 'class-missing', pattern: /(?:ClassNotFoundException|NoClassDefFoundError|Could not resolve type alias)/i,
      title: '运行产物缺少类或依赖', steps: ['查看异常链末尾的缺失类名，确认对应源码和模块依赖。', status.recovery === 'failed' ? '自动清理重建仍未成功，优先检查类包名、依赖声明和 Mapper 引用。' : '可先“重新构建”验证产物；如果仍失败，再检查类包名和依赖声明。'] },
    { id: 'compile', pattern: /(?:attempting to assign weaker access privileges|cannot find symbol|incompatible types|illegal start of|';' expected|does not override abstract method)/i,
      title: '源码编译错误', steps: ['按日志中的文件路径、行号和具体错误定位源码。', '修正访问级别、类型或语法后再构建；重复构建不会修复源码错误。'] },
    { id: 'dependency', pattern: /(?:Could not resolve dependencies|Could not (?:find|transfer) artifact|Non-resolvable parent POM|PKIX path building failed)/i,
      title: '构建依赖无法解析', steps: ['检查 Maven 仓库地址、网络、证书和本机 settings.xml。', '私服依赖需要本机配置凭据；修正后重新构建。'] },
    { id: 'node-dependency', pattern: /(?:Cannot find (?:module|package)|ERR_MODULE_NOT_FOUND|'(?:vite|vue-cli-service)' is not recognized|“(?:vite|vue-cli-service)”不是)/i,
      title: '前端依赖缺失', steps: ['在该前端目录的终端中使用项目对应的包管理器安装依赖。', '确认 package.json 脚本和 Node 版本，然后重新启动。'] },
    { id: 'proxy', pattern: /(?:ECONNREFUSED|Connection refused|连接被拒绝)/i,
      title: /http proxy error/i.test(text) ? '前端代理无法连接后端' : '依赖服务拒绝连接',
      steps: /http proxy error/i.test(text) ? ['检查后端是否已就绪，以及代理地址是否跟随后端端口。', '先查看后端日志，后端正常后再重试页面请求。'] : ['查看异常中的目标地址和端口，确认数据库、Redis 或其他依赖已启动。', '核对本机外部配置中的连接地址。'] },
    { id: 'unready', pattern: /等待就绪超过/,
      title: '进程已启动，服务尚未就绪', steps: ['进程仍在运行，可继续查看日志或停止服务。', '检查健康地址、预期状态码和启动耗时；慢启动项目可增加等待时间。'] },
  ].find(rule => rule.pattern.test(text));
  if (!rule) return null;
  return { id: rule.id, title: rule.title, steps: rule.steps,
    evidence: [...texts].reverse().find(line => rule.pattern.test(line))?.trim().slice(0, 400) || '' };
}
