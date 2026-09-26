import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import path from 'node:path';

// This fixture is emitted by TestPipelineActualSSHFirstCaptureMapCompileAndActivateWithoutPython.
// It contains an actual native Capture -> Dataset -> bindings result, not UI-invented values.
export async function testWidgetsV2(page, out) {
  const fixture=JSON.parse(readFileSync(new URL('../.cache/widget-v2-fixture.json',import.meta.url),'utf8'));
  await page.evaluate(({artifact,sample,candidate})=>{
    const id=window.__widgetSession;
    const w=window.__test.workbenches[id];
    const original=window.go.desktop.Service.WorkbenchAction;
    window.__widgetArtifacts[artifact.hash]=artifact;
    window.go.desktop.Service.WorkbenchAction=async (sid,command)=>{
      await original(sid,command);
      if(command.candidateId==='candidate-v2'&&command.kind==='widget-ready'){
        window.__widgetData=sample;
        w.panels.push({id:'widget-pipeline',title:'首采映射面板',kind:'monitor',origin:'ai',content:'',status:'实时',error:'',intervalMs:5000,updatedAt:sample.updatedAt,pinned:false,widget:{hash:artifact.hash,revision:1,bindingId:sample.bindingId,sequence:sample.sequence}});
        window.__test.emit('workbench:'+sid,structuredClone(w));
      }
    };
    w.widgetCandidates=[{...candidate,id:'candidate-v2',panelId:'widget-pipeline',expiresAt:new Date(Date.now()+30000).toISOString()}];
    window.__test.emit('workbench:'+id,structuredClone(w));
  },fixture);
  await page.waitForFunction(()=>window.__test.actions.some(a=>a.kind==='widget-ready'&&a.candidateId==='candidate-v2'));
  await page.getByRole('button',{name:'首采映射面板',exact:true}).click();
  const host=page.getByLabel('自定义实时组件');
  await host.getByText('Nginx 原始文本映射',{exact:true}).waitFor();
  assert.equal(await host.locator('.widget-metric strong').nth(0).textContent(),'2');
  assert.equal(await host.locator('.widget-metric strong').nth(1).textContent(),'30MiB');
  await host.getByRole('cell',{name:'1823',exact:true}).waitFor();
  await host.getByRole('cell',{name:'1824',exact:true}).waitFor();
  await page.screenshot({path:path.join(out,'custom-widget-v2-capture-mapping.png')});
  await page.evaluate(({candidate})=>{
    const id=window.__widgetSession,w=window.__test.workbenches[id];
    w.widgetCandidates=[{...candidate,id:'candidate-v2-no-capture',panelId:'widget-pipeline',sample:undefined,expiresAt:new Date(Date.now()+30000).toISOString()}];
    window.__test.emit('workbench:'+id,structuredClone(w));
  },fixture);
  await page.waitForFunction(()=>window.__test.actions.some(a=>a.kind==='widget-failed'&&a.candidateId==='candidate-v2-no-capture'&&a.text.includes('缺少真实')));
  assert.equal(await host.locator('.widget-metric strong').nth(1).textContent(),'30MiB');
  await page.evaluate(()=>{
    const id=window.__widgetSession,w=window.__test.workbenches[id],d=window.__widgetData;
    d.sequence++;d.data=Object.fromEntries(Object.entries(d.data).map(([key,value])=>[key,{...value,quality:'unknown',value:null,rows:null,reason:'采集超时'}]));d.issues=['capture / ps: 采集超时'];
    w.panels.find(p=>p.id==='widget-pipeline').widget.sequence=d.sequence;
    window.__test.emit('workbench:'+id,structuredClone(w));
  });
  await host.getByText('部分指标缺失：capture / ps: 采集超时',{exact:true}).waitFor();
  assert.equal(await host.locator('.widget-metric strong').nth(0).textContent(),'—');
  console.log('PASS: v2 native capture -> mapping -> real-data preflight -> bound cards/table/history; missing receipt rejected; failed data stays unknown');
}
