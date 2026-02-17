import axios from "@/api";

export const getChannelListAPI = (data: any) => {
  return axios({
    url: "/api/channel/list",
    method: "post",
    data
  });
};

export const delChannelAPI = (data: any) => {
  return axios({
    url: "/api/channel/del",
    method: "post",
    data
  });
};

export const addChannelAPI = (data: any) => {
  return axios({
    url: "/api/channel/add",
    method: "post",
    data
  });
};

export const modChannelAPI = (data: any) => {
  return axios({
    url: "/api/channel/mod",
    method: "post",
    data
  });
};
