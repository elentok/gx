#!/usr/bin/env python3
# pyright: basic
"""Cost and API responses per published ticket for gx-to-tickets authoring runs.

Run: python3 -I scripts/authoring-cost.py [--since YYYY-MM-DD] [--until YYYY-MM-DD] [--json out.json]
Reads ~/.claude/projects/*/<session>/subagents/*.jsonl and the ticket store. Read-only.
"""
import json,os,re,subprocess,sys,datetime,collections,shlex,statistics
from typing import Any
USAGE=(
"usage: python3 -I scripts/authoring-cost.py [--since YYYY-MM-DD] [--until YYYY-MM-DD] [--json out.json]\n"
"\n"
"Measures finished gx-to-tickets authoring runs of an epic: distinct API responses (distinct\n"
"message ids) and deduplicated USD cost per published ticket. gx's own actual_cost is not used:\n"
"it sums every assistant line, and one API response writes several.\n"
"Per epic: authoring runs divided by tickets without a `parent`.\n"
"\n"
"  --since DATE   skip runs that started before DATE\n"
"  --until DATE   skip runs that started after DATE\n"
"  --json FILE    also write the raw numbers to FILE\n"
"  -h, --help     print this usage and exit\n"
"\n"
"Reads ~/.claude/projects/*/<session>/subagents/*.jsonl and the ticket store. Read-only.\n"
"Baseline (36 runs to 2026-11-01): median 2.0 API responses and $0.38 per ticket, p90 3.6 and $1.22.")
ROOT=os.path.expanduser('~/.claude/projects')
PRICE={'opus':(15,75,1.5,18.75),'sonnet':(3,15,0.3,3.75),'haiku':(0.8,4,0.08,1)}
def price(model):
    m=(model or '').lower()
    for k,v in PRICE.items():
        if k in m: return v
    return PRICE['sonnet']
def cost_of(u,model):
    p=price(model)
    return (u.get('input_tokens',0)*p[0]+u.get('output_tokens',0)*p[1]+u.get('cache_read_input_tokens',0)*p[2]+u.get('cache_creation_input_tokens',0)*p[3])/1e6
def ts(s): return datetime.datetime.fromisoformat(s.replace('Z','+00:00'))
def load(rel: str) -> dict[str, Any]:
    recs=[json.loads(l) for l in open(os.path.join(ROOT,rel))]
    first=None; stamps=[]
    records=0; msgs={}; calls=[]; results={}
    for d in recs:
        if d.get('timestamp'): stamps.append(d['timestamp'])
        t=d.get('type'); m=d.get('message') or {}
        if t=='user' and first is None:
            c=m.get('content'); first=c if isinstance(c,str) else json.dumps(c)
        if t=='user' and isinstance(m.get('content'),list):
            for b in m['content']:
                if b.get('type')=='tool_result':
                    c=b.get('content'); 
                    if isinstance(c,list): c=' '.join(x.get('text','') for x in c if isinstance(x,dict))
                    results[b.get('tool_use_id')]=(c or '',bool(b.get('is_error')))
        if t=='assistant':
            records+=1
            mid=m.get('id'); msgs[mid]=(m.get('usage',{}),m.get('model'))
            if isinstance(m.get('content'),list):
                for b in m['content']:
                    if b.get('type')=='tool_use':
                        calls.append(dict(id=b['id'],name=b['name'],input=b['input'],msg=mid,ts=d.get('timestamp'),seq=len(calls)))
    msgcost={k:cost_of(u,mo) for k,(u,mo) in msgs.items()}
    msgctx={k:u.get('cache_read_input_tokens',0)+u.get('cache_creation_input_tokens',0)+u.get('input_tokens',0) for k,(u,mo) in msgs.items()}
    order=list(msgs.keys())
    cost_dedup=sum(msgcost.values())
    # gx method: sum per record (no dedupe)
    cost_rec=0
    for d in recs:
        if d.get('type')=='assistant':
            cost_rec+=cost_of(d['message'].get('usage',{}),d['message'].get('model'))
    wall=(ts(stamps[-1])-ts(stamps[0])).total_seconds() if stamps else 0
    return dict(msgcost=msgcost,msgctx=msgctx,order=order,first=first,records=records,api_calls=len(msgs),cost_dedup=cost_dedup,cost_rec=cost_rec,wall=wall,calls=calls,results=results,start=stamps[0] if stamps else '',models=sorted({mo for _,mo in msgs.values() if mo}))

SCR=re.compile(r'/scratchpad|/private/tmp|/tmp/|/\.claude/jobs/')
SETUP=re.compile(r'\.claude/skills|gx-local-tracker|SKILL\.md|docs/specs/|GLOSSARY|docs/adr|\.ai/|CLAUDE\.md|AGENTS\.md|docs/plans|docs/research')
IDENT=re.compile(r'[A-Za-z_][A-Za-z0-9_]{5,}')
STOPW={'return','string','import','export','function','const','struct','interface','strings','testing'}

def is_scratch(s):
    return bool(SCR.search(s)) and not re.search(r'/scratchpad/main(/|\s|$)',s)

def strip_heredoc(cmd):
    return re.sub(r"<<-?\s*['\"]?(\w+)['\"]?[^\n]*\n.*?\n\s*\1\b",' HEREDOC ',cmd,flags=re.S)
def segments(cmd):
    """Split on top-level && || ; newline and | (outside quotes); keep only the first command of each pipeline."""
    cmd=strip_heredoc(cmd)
    out=[];cur=[];q=None;i=0;first_done=False
    def flush():
        nonlocal cur,first_done
        t=''.join(cur).strip()
        if t and not first_done: out.append(t)
        cur=[];first_done=False
    while i<len(cmd):
        ch=cmd[i]
        if q:
            cur.append(ch)
            if ch=='\\' and q=='"' and i+1<len(cmd): cur.append(cmd[i+1]);i+=1
            elif ch==q: q=None
        elif ch in('"',"'"): q=ch;cur.append(ch)
        elif ch=='\\' and i+1<len(cmd): cur.append(ch+cmd[i+1]);i+=1
        elif cmd.startswith('&&',i) or cmd.startswith('||',i): flush();i+=1
        elif ch in(';','\n'): flush()
        elif ch=='|':
            t=''.join(cur).strip()
            if t and not first_done: out.append(t)
            cur=[];first_done=True
        else: cur.append(ch)
        i+=1
    flush()
    return out
def norm(seg):
    seg=re.sub(r'^((rtk|proxy)\s+)+','',seg.strip())
    seg=re.sub(r'^(\w+=\S*\s+)+(?=\S)','',seg)
    seg=re.sub(r'^((rtk|proxy)\s+)+','',seg)
    return seg.strip()
SHELL=('cd ','do','done','for ','then','fi','}','{','set ','export ','if ','while ','else','esac','break','trap','local','return','true','false','sort','echo','printf','read','mkdir','touch','date','pwd','sleep','exit','env ','(')
def family(seg):
    s=norm(seg)
    if not s or re.match(r'^[A-Za-z_]\w*=',s): return 'shell'
    if re.search(r'(^|\s)>>?\s*\S',s) and s.split()[0] in('cat','echo','printf','tee','sed','python3'): return 'write'
    if s.startswith(('mv ','cp ','rm ')) or re.match(r'sed\s+(-\w+\s+)*-i',s): return 'write'
    if s.startswith(SHELL): return 'shell'
    w=s.split()[0]
    if re.match(r'^(add|\w+)\(\)|^add\s',s): return 'publish'
    if w=='gx':
        if re.search(r'\s(--help|-h)(\s|$)',s) or re.match(r'gx\s+help',s): return 'cli-help'
        m=re.match(r'gx\s+tickets\s+(\w[\w-]*)',s)
        if m:
            v=m.group(1)
            if v=='validate': return 'validate'
            if v in('add','set','section','ensure-code-review','ensure-verify','epic','move','close','archive'): return 'publish'
            return 'tracker-read'
        return 'cli-run'
    if w=='go':
        if re.search(r'\btest\b',s) and re.search(r'-list\b',s): return 'go-test-list' if re.search(r"-list\s+['\"]?[^\s'\"]*Test",s) else 'go-test-scan'
        return 'go-other'
    if w=='make': return 'make-target'
    if w in('ls',):
        args=[a for a in s.split()[1:] if not a.startswith('-') and a!='2>&1' and not a.startswith('2>')]
        return 'path-probe' if args and all('/' in a and re.search(r'\.\w{1,5}\*?$',a) and not a.startswith('~') for a in args) else 'ls-dir'
    if w=='wc': return 'size'
    if w in('stat','test','[','realpath','which'): return 'path-probe'
    if w=='fd': return 'fd'
    if w=='git':
        if re.search(r'\brev-parse\b',s): return 'git-meta'
        return 'git-read'
    if w in('rg','grep'):
        if re.search(r'Makefile',s):
            return 'make-target' if re.search(r"['\"]\^",s) and not re.search(r'Makefile\s+\S',s) else 'rg'
        if re.search(r'CATALOG\.md',s):
            return 'flow-catalog' if re.search(r"['\"][^'\"]*\d{2,3}[^'\"]*['\"]",s) and not re.search(r'\s-[A-Za-z]*[AB]\d',s) else 'file-read'
        if re.search(r'\s--files(\s|$)',s): return 'fd'
        if re.search(r'\s(-[A-Za-z]*[clqL][A-Za-z]*|--count|--files-with-matches|--files-without-match|--quiet)(\s|$)',s) and not re.search(r'\s-[A-Za-z]*[nAB]',s): return 'rg-count'
        return 'rg'
    if w in('sed','cat','head','tail','awk','nl'):
        if re.search(r'CATALOG\.md',s): return 'file-read'
        return 'file-read'
    if w in('python3','python','node','bash','sh','zsh') or s.startswith(('./','$')):
        return 'publish' if re.search(r'(^|[/\s])(publish|pub)\w*\.(py|sh)\b',s) else 'script'
    if w in('wc','uniq','jq','xargs','tr','cut','paste','tee','diff','comm','column','basename','dirname','chk','check','playwright-cli'): return 'shell'
    return 'other'
def domain(seg,cwd):
    s=norm(seg)
    if SETUP.search(s): return 'setup'
    if is_scratch(s): return 'scratch'
    # no path-like token => inherits the directory of the last cd
    if not re.search(r'[\w\.\*]+/[\w\.\*]',s.split(None,1)[1] if len(s.split(None,1))>1 else ''): return cwd
    return 'code'
def rg_info(seg):
    s=norm(seg)
    try: toks=shlex.split(s)
    except ValueError: toks=s.split()
    pats=[];i=1;taken=False
    flagarg={'-g','--glob','-t','--type','-A','-B','-C','-m','--max-count','-f','-j','--sort','-r'}
    ctx=bool(re.search(r'\s-[A-Za-z]*[ABC]\b|\s-[ABC]\d|--context|--after|--before',s))
    while i<len(toks):
        t=toks[i]
        if t in('-e','--regexp') and i+1<len(toks): pats.append(toks[i+1]);taken=True;i+=2;continue
        if t in flagarg: i+=2;continue
        if t.startswith('-'): i+=1;continue
        if not taken: pats.append(t);taken=True
        i+=1
    ids={x for p in pats for x in IDENT.findall(p) if x.lower() not in STOPW}
    listing=any(re.search(r'\\w|\\S|\.\*|\^func \||\^type \||^\^?\(?func\b.{0,6}$|\^#|\^\(func',p) for p in pats) or not ids
    return ctx,listing,ids,pats
def seg_kind(seg,cwd,prov_code,prov_spec,note=None):
    f=family(seg); d=domain(seg,cwd) if f in('rg','file-read','ls-dir','fd','git-read','rg-count','path-probe','flow-catalog','size') else None
    if f in('rg','file-read','ls-dir','fd','git-read'):
        if d=='setup': return 'setup'
        if d=='scratch': return 'own-draft:check' if f=='rg' else 'own-draft:read'
    if f=='rg':
        ctx,listing,ids,pats=rg_info(seg)
        if ctx or listing: return 'learn:scan'
        prov='code' if ids and all(i in prov_code for i in ids) else 'spec' if ids and all((i in prov_code) or (i in prov_spec) for i in ids) else 'none'
        if note is not None: note.append(prov)
        decl=bool(pats) and all(re.search(r'(^|[\^\s(|])(func|type|const|var|export|class|interface|def|fn)\s',p) or re.match(r'^\^?\(?Test',p) for p in pats)
        return 'lookup:decl' if decl else 'lookup:use'
    if f=='file-read': return 'learn:read'
    if f=='size': return 'setup' if d=='setup' else 'own-draft:check' if d=='scratch' else 'learn:size'
    if f=='go-test-scan': return 'learn:scan'
    if f=='rg-count': return 'own-draft:check' if d=='scratch' else 'setup' if d=='setup' else 'lookup:count'
    if f=='ls-dir': return 'learn:ls'
    if f=='fd': return 'learn:fd'
    if f=='git-read': return 'learn:git'
    if f in('path-probe','flow-catalog','cli-help','go-test-list','make-target'):
        if d=='scratch' and f=='path-probe': return 'own-draft:read'
        if d=='setup' and f=='path-probe': return 'setup'
        return 'claim:'+f
    if f=='git-meta': return 'stamp'
    return f if f not in('shell',) else None
CLAIM_FORM_PREFIX='claim:'
def run_calls(run):
    """yield per Bash call: dict(seq,cmd,kinds,label,res_len,msg)"""
    prov_code='';prov_spec=''
    out=[]
    cwd='code';vars_={}   # the shell keeps its directory and variables between calls of one run
    # Read tool results go to provenance blobs too
    for c in run['calls']:
        res=run['results'].get(c['id'],('',0))[0]
        if c['name']=='Read':
            fp=c['input'].get('file_path','')
            if SETUP.search(fp): prov_spec+='\n'+res
            else: prov_code+='\n'+res
            continue
        if c['name']!='Bash': continue
        cmd=c['input'].get('command','')
        kinds=[];provs=[]
        for seg in segments(cmd):
            s=norm(seg)
            m=re.match(r'^([A-Za-z_]\w*)=(\S+)',s)
            if m: vars_[m.group(1)]='scratch' if is_scratch(m.group(2)) else 'setup' if SETUP.search(m.group(2)) else 'code'
            if s.startswith('cd '):
                a=s[3:].strip()
                cwd='scratch' if is_scratch(a) else vars_.get(a.lstrip('$').strip('{}'),'code') if a.startswith('$') else 'code'
                continue
            k=seg_kind(seg,cwd,prov_code,prov_spec,provs)
            if k: kinds.append(k)
        if re.search(r'<<',cmd) and not kinds: kinds.append('script')
        # label
        cl=[k for k in kinds if k.startswith('claim:')]
        lk=[k for k in kinds if k.startswith('lookup')]
        sc=[k for k in kinds if k.startswith('learn:')]
        neutral={'setup','tracker-read','stamp','validate','own-draft:check','own-draft:read'}
        if kinds and all(k.startswith('claim:') or k in('stamp',) for k in kinds) and cl: label='claim-form'
        elif (cl or lk) and not sc and all(k.startswith('claim:') or k in neutral|{'lookup:decl','lookup:use'} for k in kinds): label='claim-or-lookup'
        elif sc and not (cl or lk): label='learn'
        elif sc and (cl or lk): label='learn+claim-form' if cl else 'learn+lookup'
        elif lk: label='claim-or-lookup'
        else:
            label=next((p for p in('publish','validate','stamp','write','script','own-draft:check','own-draft:read','setup','tracker-read','cli-run','go-other') if p in kinds),'other')
            label='own-draft' if label.startswith('own-draft') else label
        out.append(dict(seq=c['seq'],cmd=cmd,kinds=kinds,provs=provs,label=label,res_len=len(res),msg=c['msg'],id=c['id']))
        if any(k.startswith(('learn','lookup','claim')) for k in kinds) and any(domain(sg,cwd)=='code' for sg in segments(cmd)): prov_code+='\n'+res
        else: prov_spec+='\n'+res
    return out

def med(v): return statistics.median(v) if v else 0
# ---------------------------------------------------------------- sample selection
PROJECTS=os.path.expanduser('~/.claude/projects')
STORE=os.path.expanduser('~/.local/share/gx/tickets/gx')
INC=re.compile(r'gx-to-tickets/SKILL\.md|`gx-to-tickets` skill via the Skill tool')
POS=re.compile(r'(?i)\b(draft\w*|publish\w*|ticket bod\w+|ticket breakdown|breakdown)\b')
NEG=re.compile(r'(?i)read-only (consultant|triage)|research ticket|write the spec|writing a spec|write a spec')
def discover():
    out=subprocess.run(['rg','-l','--no-messages','gx-to-tickets','-g','**/subagents/*.jsonl','.'],cwd=PROJECTS,capture_output=True,text=True).stdout.split()
    runs=[]
    for rel in sorted(out):
        with open(os.path.join(PROJECTS,rel)) as f:
            first=None
            for l in f:
                d=json.loads(l)
                if d.get('type')=='user':
                    c=d['message']['content']; first=c if isinstance(c,str) else json.dumps(c); break
        if first and INC.search(first) and POS.search(first[:1500]) and not NEG.search(first[:600]):
            runs.append((rel,first))
    return runs
def epic_of(first):
    m=re.search(r'docs/specs/([a-z0-9-]+)\.md',first)
    cands=[m.group(1)+'-impl'] if m else []
    cands+=re.findall(r'(?:gx:|`)([a-z0-9][a-z0-9-]*-impl)\b',first)
    for c in cands:
        if store_dir(c): return c
    return cands[0] if cands else None
def store_dir(epic):
    for base in (STORE,STORE+'/.archive'):
        d=os.path.join(base,epic,'issues')
        if os.path.isdir(d): return d
def store_count(epic):
    d=store_dir(epic) if epic else None
    if not d: return None
    n=p=0
    for f in os.listdir(d):
        if not f.endswith('.md'): continue
        head=open(os.path.join(d,f)).read().split('---')[1]
        if re.search(r'^parent:',head,re.M): p+=1
        else: n+=1
    return n,p
def published_addresses(run):
    out=set()
    for c in run['calls']:
        if c['name']!='Bash': continue
        cmd=c['input'].get('command','')
        if not re.search(r'tickets add\b|add\s*\(|publish\.(py|sh)|pub\.py|\bv\(\)',cmd): continue
        for m in re.finditer(r'gx:([a-z0-9-]+)/(\d+)',run['results'].get(c['id'],('',0))[0]): out.add(m.groups())
    return out
TAG_VERIFY=re.compile(r'verify-part|verify-collect|--type verify|genverify|verify\.py',re.I)
RENUM_OP=re.compile(r'sed\s+(-\w+\s+)*-i|\.replace\(|re\.sub\(|\bsub\(')
RENUM_ID=re.compile(r'[Tt]ickets? `?\d|@\d|\bID\[|blocked|blocked_by')
def tag_calls(run,rc):
    """Return {call id: set(tags)} for every tool call of the run."""
    tags={}
    byid={x['id']:x for x in rc}
    first_pub=min([x['seq'] for x in rc if x['label']=='publish'],default=None)
    for c in run['calls']:
        t=set(); x=byid.get(c['id']); blob=json.dumps(c['input']); cmd=c['input'].get('command','')
        if x:
            if x['label']=='claim-form': t.add('lint_lower'); t.add('lint_upper')
            elif x['label']=='claim-or-lookup': t.add('lint_upper')
            if x['label']=='publish': t.add('batch')
            elif x['label'] in('script','write') and RENUM_OP.search(cmd) and RENUM_ID.search(cmd) and 'docs/specs' not in cmd and SCR.search(cmd): t.add('batch')
            elif first_pub is not None and c['seq']>first_pub and x['label']=='tracker-read' and re.search(r'gx tickets (show|epics)',cmd): t.add('batch')
            if x['label'] in('stamp','validate'): t.add('stamps')
            elif x['kinds'] and all(k in('stamp','validate','own-draft:check') for k in x['kinds']): t.add('stamps')
        key=c['input'].get('file_path','') if c['name']=='Write' else blob
        if c['name'] in('Bash','Write','Edit') and TAG_VERIFY.search(key): t.add('verify')
        tags[c['id']]=t
    return tags
def removal_cost(run,tag):
    """first-order: share of the issuing API call's cost; second-order: result tokens re-read by later calls."""
    ids=[c for c in run['calls'] if tag in run['_tags'][c['id']]]
    per_msg=collections.Counter(c['msg'] for c in run['calls'])
    first=sum(run['msgcost'][c['msg']]/per_msg[c['msg']] for c in ids)
    order={m:i for i,m in enumerate(run['order'])}
    model=run['models'][0] if run['models'] else ''
    p=price(model)[2]
    second=0.0
    for c in ids:
        res=run['results'].get(c['id'],('',0))[0]
        later=len(run['order'])-order[c['msg']]-1
        second+=len(res)/3.5*later*p/1e6
    return first,second,len(ids)
def pct(v,p):
    v=sorted(v); 
    if not v: return 0
    k=(len(v)-1)*p; f=int(k); c=min(f+1,len(v)-1)
    return v[f]+(v[c]-v[f])*(k-f)

def arg(name):
    return sys.argv[sys.argv.index(name)+1] if name in sys.argv else None
def main():
    since,until=arg('--since'),arg('--until')
    sel=discover()
    runs=[]
    for rel,first in sel:
        r=load(rel); r['p']=rel; r['epic']=epic_of(first)
        if since and r['start'][:10]<since: continue
        if until and r['start'][:10]>until: continue
        rc=run_calls(r); r['_rc']=rc; r['_tags']=tag_calls(r,rc)
        r['published']=len(published_addresses(r))
        runs.append(r)
    res: dict[str, Any] = {'n_runs': len(runs)}
    rows: list[dict[str, Any]] = []
    for r in runs:
        rows.append(dict(p=r['p'],epic=r['epic'],records=r['records'],api_calls=r['api_calls'],bash=len(r['_rc']),
            cost_dedup=round(r['cost_dedup'],2),cost_gx_method=round(r['cost_rec'],2),wall_min=round(r['wall']/60,1),
            published=r['published'],model=','.join(r['models']),labels=dict(collections.Counter(x['label'] for x in r['_rc']))))
    res['runs'] = rows
    out = arg('--json')
    if out:
        with open(out, 'w') as f:
            json.dump(res, f, indent=1)
    return runs,res
def report(runs):
    P=print
    P('## sample'); P('runs',len(runs),'from',min(r['start'] for r in runs)[:10],'to',max(r['start'] for r in runs)[:10],'models',dict(collections.Counter(m for r in runs for m in r['models'])))
    for name,key in(('records (assistant lines)','records'),('API calls (distinct message ids)','api_calls'),('Bash calls',None),('cost USD dedup',None),('cost USD gx method',None),('wall min',None)):
        v=[r[key] for r in runs] if key else [len(r['_rc']) for r in runs] if name=='Bash calls' else [r['cost_dedup'] for r in runs] if 'dedup' in name else [r['cost_rec'] for r in runs] if 'gx method' in name else [r['wall']/60 for r in runs]
        P('%-34s n=%d median %.1f p90 %.1f max %.1f total %.1f'%(name,len(v),med(v),pct(v,.9),max(v),sum(v)))
    P('## publishing runs (created tickets with gx tickets add in the run)')
    pub=[r for r in runs if r['published']>0]
    for r in pub:
        n=r['published']
        P('%-30s tickets %3d records %4d (%.1f/t) api %4d (%.1f/t) cost %.2f dedup (%.3f/t) %.2f gx (%.3f/t) wall %.0fm'%(r['epic'],n,r['records'],r['records']/n,r['api_calls'],r['api_calls']/n,r['cost_dedup'],r['cost_dedup']/n,r['cost_rec'],r['cost_rec']/n,r['wall']/60))
    if pub:
        for nm,f in(('records/ticket',lambda r:r['records']/r['published']),('api/ticket',lambda r:r['api_calls']/r['published']),('$dedup/ticket',lambda r:r['cost_dedup']/r['published']),('$gx/ticket',lambda r:r['cost_rec']/r['published'])):
            v=[f(r) for r in pub]; P('  %-16s n=%d median %.3f p90 %.3f'%(nm,len(v),med(v),pct(v,.9)))
    P('## per epic (all authoring runs of the epic / tickets in the store without parent)')
    by=collections.defaultdict(list)
    for r in runs: by[r['epic']].append(r)
    ep=[]
    for e,rs in by.items():
        sc=store_count(e)
        if not sc: P('  %-30s runs %d: no epic in store'%(e,len(rs))); continue
        n=sc[0]; rec=sum(r['records'] for r in rs); api=sum(r['api_calls'] for r in rs); c=sum(r['cost_dedup'] for r in rs); cg=sum(r['cost_rec'] for r in rs)
        ep.append((e,len(rs),n,rec/n,api/n,c/n,cg/n))
        P('  %-30s runs %d tickets %3d records/t %.1f api/t %.1f $dedup/t %.3f $gx/t %.3f'%(e,len(rs),n,rec/n,api/n,c/n,cg/n))
    for i,nm in((3,'records/ticket'),(4,'api/ticket'),(5,'$dedup/ticket'),(6,'$gx/ticket')):
        v=[x[i] for x in ep]; P('  %-16s n=%d median %.3f p90 %.3f'%(nm,len(v),med(v),pct(v,.9)))
    P('## bash labels (calls)'); lc=collections.Counter(x['label'] for r in runs for x in r['_rc']); tot=sum(lc.values())
    for k,v in lc.most_common(): P('  %-20s %5d %5.1f%%'%(k,v,100*v/tot))
    P('## segment kinds'); kc=collections.Counter(k for r in runs for x in r['_rc'] for k in x['kinds'])
    for k,v in kc.most_common(): P('  %-20s %5d'%(k,v))
    P('## candidates: removable calls and first/second-order cost')
    totcost=sum(r['cost_dedup'] for r in runs); totbash=tot; totapi=sum(r['api_calls'] for r in runs)
    for tag in('lint_lower','lint_upper','batch','verify','stamps'):
        f=s2=0;n=0;nb=0
        for r in runs:
            a,b,k=removal_cost(r,tag); f+=a;s2+=b;n+=k
            nb+=sum(1 for c in r['calls'] if c['name']=='Bash' and tag in r['_tags'][c['id']])
        P('  %-11s calls %4d (bash %4d = %.1f%% of bash, %.1f%% of API calls) first-order $%.2f (%.1f%%) second-order $%.2f total %.1f%% of $%.2f'%(tag,n,nb,100*nb/totbash,100*n/totapi,f,100*f/totcost,s2,100*(f+s2)/totcost,totcost))
    # union
    f=s2=0;n=0
    for r in runs:
        for c in r['calls']:
            pass
    P('## non-bash'); P('  tools',collections.Counter(c['name'] for r in runs for c in r['calls']).most_common(8))

# Precision of each segment kind, from the hand samples in the research doc (claim = the call checks a named item).
PRECISION={'claim':26/30,'lookup:decl':11/25,'lookup:use':2/30,'lookup:count':2/10}
def lint_expected(runs):
    """Expected number (and first-order cost) of lint-removable calls: a call goes only if every segment is a claim."""
    n=cost=0.0
    for r in runs:
        per_msg=collections.Counter(c['msg'] for c in r['calls'])
        for x in r['_rc']:
            if x['label'] not in('claim-form','claim-or-lookup'): continue
            p=1.0
            for k in x['kinds']:
                if k.startswith('claim:'): p*=PRECISION['claim']
                elif k in PRECISION: p*=PRECISION[k]
            n+=p; cost+=p*r['msgcost'][x['msg']]/per_msg[x['msg']]
    return n,cost
def extra(runs):
    P=print
    P('## extra')
    n,c=lint_expected(runs); tb=sum(len(r['_rc']) for r in runs); tc=sum(r['cost_dedup'] for r in runs)
    P('  lint expected calls %.1f (%.1f%% of bash) first-order $%.2f (%.1f%%)'%(n,100*n/tb,c,100*c/tc))
    simple=0;allc=0
    for r in runs:
        for x in r['_rc']:
            allc+=1
            words=[norm(sg).split()[0] for sg in segments(x['cmd']) if norm(sg) and family(sg)!='shell' and norm(sg).split()]
            if words and all(w in('rg','ls','sed','cat') for w in words): simple+=1
    P('  calls made only of rg/ls/sed/cat segments: %d of %d = %.1f%%'%(simple,allc,100*simple/allc))
    anyw=0
    for r in runs:
        for x in r['_rc']:
            words=[norm(sg).split()[0] for sg in segments(x['cmd']) if norm(sg) and norm(sg).split()]
            if any(w in('rg','ls','sed','cat') for w in words): anyw+=1
    P('  calls with at least one rg/ls/sed/cat segment: %d = %.1f%%'%(anyw,100*anyw/allc))
    helps=collections.Counter()
    for r in runs:
        for x in r['_rc']:
            for sg in segments(x['cmd']):
                if family(sg)=='cli-help':
                    w=[t for t in norm(sg).split() if not t.startswith('-') and t!='2>&1'][:4]
                    helps[' '.join(w)]+=1
    P('  distinct --help commands %d, total %d; top %s'%(len(helps),sum(helps.values()),helps.most_common(6)))
    # gopls
    P('  gopls symbol-reference calls (not Bash):',sum(1 for r in runs for c in r['calls'] if c['name'].startswith('mcp__gopls')))
    # per model
    for mo in ('claude-opus-5-5','claude-sonnet-5-5'):
        rs=[r for r in runs if mo in r['models']]
        P('  %s runs %d records median %.0f api median %.0f cost dedup median %.2f total %.1f'%(mo,len(rs),med([r['records'] for r in rs]),med([r['api_calls'] for r in rs]),med([r['cost_dedup'] for r in rs]),sum(r['cost_dedup'] for r in rs)))
    # reread of file-read segments
    seen={};tot=rr=0
    for r in runs:
        seen={}
        for x in r['_rc']:
            for sg in segments(x['cmd']):
                if family(sg)!='file-read': continue
                s=norm(sg)
                mm=re.search(r"sed\s+-n\s+'?(\d+),(\d+)p'?\s+(\S+)",s)
                if mm:
                    a,b,p=int(mm.group(1)),int(mm.group(2)),mm.group(3)
                    tot+=1
                    if any(a>=x0 and b<=y0 for x0,y0 in seen.get(p,[])): rr+=1
                    seen.setdefault(p,[]).append((a,b))
    P('  sed -n ranges: %d, fully inside an earlier range of the same file in the same run: %d'%(tot,rr))
    # phase: bash calls before the first Write/publish
    pre=[];post=[]
    for r in runs:
        fw=min([c['seq'] for c in r['calls'] if c['name']=='Write'],default=None)
        n_pre=sum(1 for x in r['_rc'] if fw is None or x['seq']<fw)
        pre.append(n_pre); post.append(len(r['_rc'])-n_pre)
    P('  bash calls before the first Write: median %.0f, after: median %.0f; share before %.1f%%'%(med(pre),med(post),100*sum(pre)/(sum(pre)+sum(post))))

def union_and_table(runs):
    P=print
    f=0.0;n=0;tc=sum(r['cost_dedup'] for r in runs); tb=sum(len(r['_rc']) for r in runs)
    f_low=0.0;n_low=0
    for r in runs:
        per_msg=collections.Counter(c['msg'] for c in r['calls'])
        for c in r['calls']:
            t=r['_tags'][c['id']]
            if t&{'lint_upper','batch','verify','stamps'}: n+=1; f+=r['msgcost'][c['msg']]/per_msg[c['msg']]
            if t&{'lint_lower','batch','verify','stamps'}: n_low+=1; f_low+=r['msgcost'][c['msg']]/per_msg[c['msg']]
    P('## union of the four candidates (each call counted once): upper %d calls, first-order $%.2f (%.1f%% of $%.2f); with lint lower bound %d calls $%.2f (%.1f%%)'%(n,f,100*f/tc,tc,n_low,f_low,100*f_low/tc))
    P('## table')
    P('| run | epic | model | records | API calls | Bash | $ (dedup) | $ (gx method) | wall min | tickets created in run | tickets in epic |')
    P('|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|')
    for r in sorted(runs,key=lambda r:r['start']):
        sc=store_count(r['epic']) if r['epic'] else None
        P('| %s | %s | %s | %d | %d | %d | %.2f | %.2f | %.0f | %s | %s |'%(r['p'].split('agent-')[-1][:8],r['epic'],r['models'][0].replace('claude-','').replace('-5-5',''),r['records'],r['api_calls'],len(r['_rc']),r['cost_dedup'],r['cost_rec'],r['wall']/60,r['published'] or '-',sc[0] if sc else '-'))

if __name__=='__main__':
    if '--help' in sys.argv or '-h' in sys.argv:
        print(USAGE); sys.exit(0)
    runs,res=main()
    report(runs); extra(runs); union_and_table(runs)
