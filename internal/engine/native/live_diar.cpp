#include "nemo_speech/diar.h"
#include <cstdio>
#include <cstdint>
#include <vector>

int fail(const char* where) {
 std::fprintf(stderr,"%s: %s\n",where,nemo_speech_asr_last_error());return 1;
}
int main(int argc,char** argv) {
 if(argc!=2)return 2;
 nemo_speech_diar_model_config cfg{};cfg.size=sizeof(cfg);cfg.model_path=argv[1];cfg.gpu=0;cfg.preset="v3-streaming";
 nemo_speech_diar_model* model=nullptr;
 if(nemo_speech_diar_create(&cfg,&model)!=NEMO_SPEECH_ASR_OK)return fail("create");
 nemo_speech_diar_stream* stream=nullptr;
 if(nemo_speech_diar_stream_open(model,&stream)!=NEMO_SPEECH_ASR_OK)return fail("open");
 auto emit=[&]() {
  size_t count=0;
  if(nemo_speech_diar_segments(stream,nullptr,nullptr,0,&count)!=NEMO_SPEECH_ASR_OK)return false;
  std::vector<nemo_speech_diar_segment> segs(count);
  if(count&&nemo_speech_diar_segments(stream,nullptr,segs.data(),count,&count)!=NEMO_SPEECH_ASR_OK)return false;
  std::printf("{\"segments\":[");
  for(size_t i=0;i<count;i++)std::printf("%s{\"start\":%.3f,\"end\":%.3f,\"speaker\":%d}",i?",":"",segs[i].start_time,segs[i].end_time,segs[i].speaker);
  std::printf("]}\n");std::fflush(stdout);return true;
 };
 std::puts("{\"ready\":true}");std::fflush(stdout);
 int16_t pcm[2560];float samples[2560];size_t total=0,next=32000;
 while(size_t n=std::fread(pcm,sizeof(int16_t),2560,stdin)) {
  for(size_t i=0;i<n;i++)samples[i]=pcm[i]/32768.0f;
  if(nemo_speech_diar_stream_push_f32(stream,samples,n,16000)!=NEMO_SPEECH_ASR_OK)return fail("push");
  total+=n;
  if(total>=next){if(!emit())return fail("segments");next=total+32000;}
 }
 if(std::ferror(stdin))return 1;
 if(nemo_speech_diar_stream_finish(stream)!=NEMO_SPEECH_ASR_OK)return fail("finish");
 if(!emit())return fail("segments");
 nemo_speech_diar_stream_close(stream);nemo_speech_diar_destroy(model);
 return 0;
}
