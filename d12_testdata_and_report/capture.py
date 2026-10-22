
import datetime,fcntl,json,os,pathlib,select,signal,struct,termios,time,tty
from decimal import Decimal
folder=pathlib.Path(__file__).parent
started=time.monotonic()
fd=None
original=None
received=frames=invalid=0
reason='timeout'
weights=set()
states=set()
candidate_matches=0

def stamp():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()

def stop(sig,frame):
    raise SystemExit('signal '+str(sig))

signal.signal(signal.SIGTERM,stop)
signal.signal(signal.SIGINT,stop)

def uart():
    b=bytearray(80)
    fcntl.ioctl(fd,0x545D,b,True)
    v=struct.unpack('20i',b)
    return dict(zip(['rx','tx','frame','overrun','parity'],[v[4],v[5],v[6],v[7],v[8]]))

try:
    fd=os.open('/dev/ttyS0',os.O_RDWR|os.O_NOCTTY|os.O_NONBLOCK)
    original=termios.tcgetattr(fd)
    fcntl.ioctl(fd,termios.TIOCEXCL)
    tty.setraw(fd,termios.TCSANOW)
    a=termios.tcgetattr(fd)
    a[2]|=termios.CLOCAL|termios.CREAD
    a[2]&=~(termios.PARENB|termios.PARODD|termios.CSTOPB|termios.CRTSCTS)
    a[4]=a[5]=termios.B9600
    termios.tcsetattr(fd,termios.TCSANOW,a)
    termios.tcflush(fd,termios.TCIFLUSH)
    initial=uart()
    metadata={'startedAt':stamp(),'pid':os.getpid(),'device':'/dev/ttyS0','baud':9600,'format':'8N1','flow':'none','protocol':'D12 TF05','durationSeconds':1200,'receiveOnly':True,'uartStart':initial}
    (folder/'metadata.json').write_text(json.dumps(metadata,ensure_ascii=False,indent=2))
    pending=bytearray()
    with (folder/'raw.bin').open('wb',buffering=0) as raw,(folder/'chunks.jsonl').open('w',buffering=1) as chunks,(folder/'frames.jsonl').open('w',buffering=1) as output:
        (folder/'ready.json').write_text(json.dumps({'state':'recording','startedAt':metadata['startedAt'],'pid':os.getpid()}))
        print('READY',str(folder),flush=True)
        deadline=time.monotonic()+1200
        heartbeat=time.monotonic()+10
        while time.monotonic()<deadline:
            if select.select([fd],[],[],min(0.5,max(0,deadline-time.monotonic())))[0]:
                data=os.read(fd,4096)
                if not data:
                    reason='serial_eof'
                    break
                now=stamp()
                chunks.write(json.dumps({'receivedAt':now,'offset':received,'hex':data.hex(' ')})+'\n')
                raw.write(data)
                received+=len(data)
                pending.extend(data)
                while True:
                    pos=pending.find(b'\x02')
                    if pos<0:
                        invalid+=len(pending)
                        pending.clear()
                        break
                    if pos:
                        invalid+=pos
                        del pending[:pos]
                    if len(pending)<18:break
                    f=bytes(pending[:18])
                    if f[16]!=13 or not f[4:16].isdigit():
                        invalid+=1
                        del pending[0]
                        continue
                    del pending[:18]
                    a,b,c=f[1:4]
                    value=Decimal(int(f[4:10])).scaleb(2-(a&7))
                    if b&2:value=-value
                    tare=Decimal(int(f[10:16])).scaleb(2-(a&7))
                    candidate=sum(f[:16])&255
                    row={'receivedAt':now,'index':frames+1,'rawHex':f.hex(' '),'statusA':a,'statusB':b,'statusC':c,'displayWeight':str(value),'tare':str(tare),'unitFlag':'kg' if b&16 else 'unknown','weightType':'net' if b&1 else 'gross','negative':bool(b&2),'dynamic':bool(b&8),'stable':not bool(b&8),'overloadOrBelowZero':bool(b&4),'checksumByte':f[17],'candidateSumNoCR':candidate,'candidateMatches':candidate==f[17],'checksumStatus':'unconfirmed_algorithm'}
                    output.write(json.dumps(row,ensure_ascii=False)+'\n')
                    frames+=1
                    weights.add(str(value))
                    states.add((bool(b&8),bool(b&4),bool(b&1)))
                    candidate_matches+=int(candidate==f[17])
            if time.monotonic()>=heartbeat:
                print(json.dumps({'at':stamp(),'bytes':received,'frames':frames,'uniqueWeights':len(weights),'states':sorted(states),'candidateMatches':candidate_matches}),flush=True)
                heartbeat=time.monotonic()+10
        if pending:
            (folder/'trailing-partial.hex').write_text(pending.hex(' '))
except BaseException as exc:
    reason=type(exc).__name__+': '+str(exc)
    print('STOP',reason,flush=True)
finally:
    counts=None
    if fd is not None:
        try:counts=uart()
        except OSError:pass
        try:
            if original is not None:termios.tcsetattr(fd,termios.TCSANOW,original)
        finally:os.close(fd)
    summary={'endedAt':stamp(),'reason':reason,'durationSeconds':round(time.monotonic()-started,3),'bytes':received,'frames':frames,'discardedBytes':invalid,'uniqueWeights':len(weights),'states':sorted(states),'candidateMatches':candidate_matches,'uartEnd':counts}
    (folder/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2))
    (folder/'ready.json').write_text(json.dumps({'state':'stopped','endedAt':summary['endedAt'],'reason':reason}))
    print('FINISHED',json.dumps(summary),flush=True)
