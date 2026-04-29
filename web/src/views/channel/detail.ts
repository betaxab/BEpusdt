import { ref } from "vue";

export interface ChannelDetail {
  id: number;
  name: string;
  qrcode: string;
  config: string;
  trade_type: string;
  remark: string;
  other_notify: number;
  status: number;
  created_at?: string;
  updated_at?: string;
}

export const useChannelDetail = () => {
  const detailVisible = ref(false);
  const detailData = ref<ChannelDetail>({
    id: 0,
    name: "",
    qrcode: "",
    config: "",
    trade_type: "",
    remark: "",
    other_notify: 0,
    status: 0
  });

  // 显示详情
  const showDetail = (record: ChannelDetail) => {
    detailData.value = { ...record };
    detailVisible.value = true;
  };

  // 关闭详情
  const closeDetail = () => {
    detailVisible.value = false;
    detailData.value = {
      id: 0,
      name: "",
      qrcode: "",
      config: "",
      trade_type: "",
      remark: "",
      other_notify: 0,
      status: 0
    };
  };

  return {
    detailVisible,
    detailData,
    showDetail,
    closeDetail
  };
};
