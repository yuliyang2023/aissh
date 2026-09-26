// This entire module executes inside QuickJS, never in the browser realm.
export const Fragment = Symbol('fragment');
let callbacks = new Map(), nextID = 0, rendering = false, state, definition, frame, current;
const stringify = JSON.stringify.bind(JSON);
const parse = JSON.parse.bind(JSON);
const ownEntries = Object.entries;
export function defineWidget(value) { return value; }
export function jsx(type, props = {}) {
  if (type === Fragment) return props.children;
  if (typeof type === 'function') return type(props);
  return {type, props};
}
export const jsxs = jsx;
export const jsxDEV = jsx;
function builtin(type) { return (props) => jsx(type, props); }
export const MetricCard = builtin('MetricCard');
export const StatusBadge = builtin('StatusBadge');
export const LineChart = builtin('LineChart');
export const BarChart = builtin('BarChart');
export const DataTable = builtin('DataTable');
function json(value, limit) { const s=stringify(value); if (!s || s.length>limit) throw Error('组件数据超过限制'); return parse(s); }
function setState(value) {
  if (rendering) throw Error('render 中不能调用 setState');
  state=json(typeof value==='function'?value(state):value,16384);
}
function normalize(input, depth=0, budget={count:0}) {
  if(depth>32 || ++budget.count>2000) throw Error('组件节点超过限制');
  if(input==null || typeof input==='boolean') return [];
  if(Array.isArray(input)) return input.flatMap(v=>normalize(v,depth+1,budget));
  if(typeof input==='string'||typeof input==='number') return [{kind:'text',text:String(input)}];
  if(typeof input!=='object'||typeof input.type!=='string') throw Error('无效组件节点');
  const props={}, events={};let children=[];
  for(const [key,value] of ownEntries(input.props||{})) {
    if(key==='children') children=normalize(value,depth+1,budget);
    else if(key==='key') continue;
    else if(/^on[A-Z]/.test(key)) {
      if(typeof value!=='function') throw Error('事件必须是函数');
      const id=String(++nextID); callbacks.set(id,value);events[key]=id;
    } else props[key]=value;
  }
  return [{kind:'element',type:input.type,props,events,children}];
}
export function __init(d) {
 if(typeof d==='function') d={render:d};
 definition=d;
 if(!d || typeof d.render!=='function') throw Error('组件入口无效：请 default export defineWidget({ initialState, render }) 或普通渲染函数');
 state=json(d.initialState??{},16384);
}
export function __render(raw) {
 const data=parse(raw);current=data;callbacks.clear();rendering=true;
 try {
  const history={series(field,options={}) {
   const minutes=Math.max(1,Math.min(10,Number(options.minutes)||10));
   const end=Date.parse(data.updatedAt)||Date.now();
   return (data.history||[]).filter(p=>Date.parse(p.time)>=end-minutes*60000).map(p=>({time:p.time,value:p.values[field]??null}));
  }};
  frame=normalize(definition.render({data:data.data,lastGoodSample:{data:data.lastGood,updatedAt:data.lastGoodAt},history,state,setState,theme:data.theme||{}}));
  const output=stringify(frame);if(output.length>262144) throw Error('组件输出过大');return output;
 } finally {rendering=false;}
}
export function __event(id,payload) {
 const fn=callbacks.get(id);if(!fn) throw Error('交互已过期');fn(parse(payload));
 return __render(stringify(current));
}
