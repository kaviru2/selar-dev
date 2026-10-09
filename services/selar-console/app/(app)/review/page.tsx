"use client";
import {useEffect,useState} from 'react';
import {PracticeCard,practiceRPC,type PracticeItem} from '@/components/ReadingPractice';
import {buttonClass} from '@/components/ui/Button';
import {PageHeader} from '@/components/ui/Card';
export default function DailyReview(){
 const [items,setItems]=useState<PracticeItem[]>([]);const [streak,setStreak]=useState(0);const [loading,setLoading]=useState(true);const [error,setError]=useState('');const [done,setDone]=useState<string[]>([]);
 async function load(){setLoading(true);setError('');try{const data=await practiceRPC({action:'daily'}) as {items:PracticeItem[];streak:number};setItems(data.items);setStreak(data.streak);}catch{setError('Daily review unavailable; retry.');}finally{setLoading(false);}}
 useEffect(()=>{void load();},[]);
 const remaining=items.filter(i=>!done.includes(i.id));
 return <main className="practice-page"><div className="practice-inner">
 <PageHeader eyebrow="Review" title="Daily review" description={<>{streak}-day review streak · UTC calendar days</>}/><p className="practice-lede">Recall before opening your documents. Mark source/hint use honestly. Schedule: conservative doubling intervals, not a calibrated memory model.</p>
 {loading?<p className="practice-status">Loading due practice…</p>:error?<p role="alert" className="practice-error">{error} <button type="button" className={buttonClass({size:'sm'})} onClick={load}>Retry</button></p>:<>
 {!remaining.length&&<p role="status" className="practice-status">Nothing due. {done.length?'Session complete.':'Finish a reading check to schedule tomorrow’s practice.'}</p>}
 {remaining.slice(0,1).map(item=><PracticeCard key={item.id} item={item} phase="review" onDone={()=>{void practiceRPC({action:'daily'}).then(data=>setStreak((data as unknown as {streak:number}).streak)).catch(()=>undefined);}}/>)}
 {remaining.length>0&&<div className="practice-actions"><button type="button" className={buttonClass({size:'sm'})} onClick={()=>setDone([...done,remaining[0].id])}>Next question</button></div>}
 <p className="practice-status">{done.length} of {items.length} questions visited this session</p></>}
 <p className="practice-links"><a className="practice-link" href="/reader">Read documents</a> · <a className="practice-link" href="/progress">Practice progress</a></p>
 </div></main>;
}
