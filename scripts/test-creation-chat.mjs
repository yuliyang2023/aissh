// Run after frontend build with PLAYWRIGHT_MODULE and UI_BROWSER_PATH if needed.
// This exercises the New dialogs through an in-memory desktop bridge.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile, mkdir } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE || 'playwright');

test('New dialogs create local data through independent AI conversations', { timeout: 90000 }, async () => {
  const browser = await chromium.launch({ executablePath: process.env.UI_BROWSER_PATH });
  try {
    await mkdir(path.join(root, '.cache/creation-ui'), { recursive: true });
    const page = await browser.newPage({ viewport: { width: 1280, height: 860 } });
    page.setDefaultTimeout(10000);
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.route('http://creation.test/**', async route => {
      const pathname = new URL(route.request().url()).pathname;
      const file = path.join(root, 'frontend/dist', pathname === '/' ? 'index.html' : pathname);
      const body = await readFile(file);
      await route.fulfill({ body, contentType: file.endsWith('.js') ? 'text/javascript' : file.endsWith('.css') ? 'text/css' : 'text/html' });
    });
    await page.addInitScript(() => {
      const data = { version: 1, revision: 1, hosts: [], credentials: [], resources: [],
        aiProfiles: [{id:'ai',name:'默认资料助手',provider:'model'}], defaultAIProfileId:'ai' };
      const knowledge = [], conversations = {};
      window.creationState = { data, knowledge, requests: [], manualSaves: 0, sshCalls: 0 };
      const events = {};
      window.runtime = { EventsOn(name, callback) { events[name] = callback; return () => delete events[name]; } };
      const service = {
        Snapshot:async () => structuredClone(data), ListConversations:async()=>[], ListSessions:async()=>[],
        ListKnowledge:async()=>structuredClone(knowledge),
        SaveKnowledge:async()=>{window.creationState.manualSaves++;throw Error('manual save not expected');},
        SaveResource:async()=>{window.creationState.manualSaves++;throw Error('manual save not expected');},
        CreateSession:async()=>{window.creationState.sshCalls++;}, StartSession:async()=>{window.creationState.sshCalls++;},
        AskCatalogAssistant:async input => {
          window.creationState.requests.push(input);
          const messages = conversations[input.conversationId] ||= [];
          messages.push({role:'user',content:input.prompt,time:new Date().toISOString()});
          events['catalog-create:' + input.conversationId]?.('正在整理资料…');
          await new Promise(resolve => setTimeout(resolve, 180));
          let receipts = [], error = '';
          if (input.mode === 'system' && messages.length === 1) {
            messages.push({role:'assistant',content:'请提供系统的服务地址。'});
          } else if (input.mode === 'system') {
            data.resources.push({id:'system-one',name:'CMDB',kind:'http',endpoint:'https://cmdb.example.com',description:'资产管理',grant:'ask',route:'local',credentialId:'',hostId:'',database:'',tls:false,allowLocal:false});
            receipts = [{id:'system-one',kind:'system',title:'CMDB',status:'saved',detail:'CMDB · https://cmdb.example.com'}];
          } else {
            const existing = knowledge.length > 0;
            if (!existing) knowledge.push({id:'data-one',title:'资产归属',kind:'data',columns:['IP','部门'],rows:[['192.0.2.10','研发部']],tags:['AI 整理'],content:'',hostId:'',resourceId:'',sourceUrl:'',source:'',filePath:'',conversationId:'',updatedAt:new Date().toISOString(),createdAt:new Date().toISOString(),changes:[{time:new Date().toISOString(),quote:input.prompt,values:{IP:'192.0.2.10',部门:'研发部'}}]});
            receipts = [{id:'data-one',kind:'knowledge',title:'资产归属',status:existing?'unchanged':'created',row:1,detail:'资产归属 · 第 1 条记录'}];
            if (input.prompt.includes('返回异常')) error = '模拟：资料已保存，AI 回复中断';
          }
          if (receipts.length) messages.push({role:'system',content:'已保存：' + receipts[0].detail});
          if (error) messages.push({role:'error',content:error});
          else if (receipts.length) messages.push({role:'assistant',content:'信息已整理并保存。'});
          return {messages:structuredClone(messages),receipts,error};
        },
      };
      window.go = {desktop:{Service:service}};
    });
    await page.goto('http://creation.test/');
    await page.getByRole('button',{name:'本地知识',exact:true}).click();
    await page.getByRole('button',{name:'新建知识',exact:true}).click();
    await page.getByLabel('标题',{exact:true}).fill('保留的表单草稿');
    await page.getByRole('button',{name:'与 AI 对话创建',exact:true}).click();
    const prompt = page.getByRole('textbox',{name:'告诉 AI 要保存的信息',exact:true});
    await prompt.fill('192.0.2.10 属于研发部');
    await page.getByRole('button',{name:'发送',exact:true}).click();
    assert.equal(await page.getByRole('button',{name:'填写表单',exact:true}).isDisabled(),true);
    await page.locator('.creation-receipt').waitFor();
    assert.equal(await page.evaluate(()=>window.creationState.knowledge.length),1);
    assert.equal(await page.evaluate(()=>window.creationState.manualSaves),0);
    await page.getByRole('button',{name:'填写表单',exact:true}).click();
    assert.equal(await page.getByLabel('标题',{exact:true}).inputValue(),'保留的表单草稿');
    await page.getByRole('button',{name:'与 AI 对话创建',exact:true}).click();
    await prompt.fill('192.0.2.10 属于研发部');
    await prompt.press('Enter');
    await page.getByText('已存在，无需重复创建',{exact:true}).waitFor();
    assert.equal(await page.evaluate(()=>window.creationState.knowledge.length),1);
    await page.screenshot({path:path.join(root,'.cache/creation-ui/knowledge-chat.png')});
    await page.getByRole('button',{name:'完成',exact:true}).click();
    await page.getByRole('button',{name:'资产归属',exact:true}).click();
    await page.getByText('对话保存记录（1）',{exact:true}).click();
    await page.getByText('IP：192.0.2.10 · 部门：研发部',{exact:true}).waitFor();
    await page.getByRole('button',{name:'关闭窗口',exact:true}).click();

    await page.getByRole('button',{name:'外联系统',exact:true}).click();
    await page.getByRole('button',{name:'添加系统',exact:true}).click();
    await page.getByRole('button',{name:'与 AI 对话创建',exact:true}).click();
    await prompt.fill('添加 CMDB，类型 HTTP，用于资产管理');
    await page.getByRole('button',{name:'发送',exact:true}).click();
    await page.getByText('请提供系统的服务地址。',{exact:true}).waitFor();
    assert.equal(await page.evaluate(()=>window.creationState.data.resources.length),0);
    await prompt.fill('地址是 https://cmdb.example.com');
    await page.getByRole('button',{name:'发送',exact:true}).click();
    await page.locator('.creation-receipt').waitFor();
    await page.setViewportSize({width:940,height:680});
    await page.emulateMedia({colorScheme:'dark'});
    await page.screenshot({path:path.join(root,'.cache/creation-ui/system-chat-dark.png')});
    const bounds = await page.getByRole('button',{name:'完成',exact:true}).boundingBox();
    assert.ok(bounds && bounds.y + bounds.height <= 680, 'finish action fits smaller window');
    await page.getByRole('button',{name:'关闭窗口',exact:true}).click();
    await page.getByText('CMDB',{exact:true}).waitFor();
    assert.equal(await page.evaluate(()=>window.creationState.sshCalls),0);
    assert.equal(await page.evaluate(()=>window.creationState.manualSaves),0);
    const requests = await page.evaluate(()=>window.creationState.requests);
    assert.equal(requests[0].conversationId,requests[1].conversationId);
    assert.equal(requests[2].conversationId,requests[3].conversationId);
    assert.notEqual(requests[0].conversationId,requests[2].conversationId);
    assert.deepEqual(errors,[]);
  } finally { await browser.close(); }
});
